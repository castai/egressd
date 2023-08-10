package dns

import (
	"context"
	"fmt"
	"net/http"
	"time"

	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/google/gopacket/layers"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"inet.af/netaddr"

	"github.com/castai/egressd/ebpf"
	"github.com/castai/egressd/pb"
)

type LookuperStarter interface {
	Start(ctx context.Context) error
	Lookup(ip netaddr.IP) string
	GetIp2DnsHandler(http.ResponseWriter, *http.Request)
}

var _ LookuperStarter = (*IP2DNS)(nil)

type tracer interface {
	Run(ctx context.Context) error
	Events() <-chan ebpf.DNSEvent
}

var defaultDNSTTL = 5 * time.Minute

type IP2Domain struct {
	IP     string
	Domain string
}

type IP2DNS struct {
	Tracer      tracer
	log         logrus.FieldLogger
	ipToName    *cache.Cache[string, string]
	cnameToName *cache.Cache[string, string]
}

func NewIP2DNS(tracer tracer, log logrus.FieldLogger) *IP2DNS {
	return &IP2DNS{
		Tracer: tracer,
		log:    log,
	}
}

func (d *IP2DNS) GetIp2DnsHandler(w http.ResponseWriter, req *http.Request) {
	items := make([]*pb.IP2Domain, 0, len(d.ipToName.Keys()))
	for _, ip := range d.ipToName.Keys() {
		domain, _ := d.ipToName.Get(ip)
		items = append(items, &pb.IP2Domain{Ip: ip, Domain: domain})
	}
	batch := &pb.IP2DomainBatch{Items: items}
	batchBytes, err := proto.Marshal(batch)
	if err != nil {
		d.log.Errorf("marshal batch: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(batchBytes); err != nil {
		d.log.Errorf("write batch: %v", err)
		return
	}
	d.ipToName.DeleteExpired()
	d.cnameToName.DeleteExpired()
}

func (d *IP2DNS) Start(ctx context.Context) error {

	d.ipToName = cache.NewContext[string, string](ctx)
	d.cnameToName = cache.NewContext[string, string](ctx)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errch := make(chan error, 1)
	go func() {
		err := d.Tracer.Run(ctx)
		errch <- err
	}()

	evCh := d.Tracer.Events()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errch:
			if err != nil {
				return fmt.Errorf("running tracer: %w", err)
			}
			return nil
		case ev, ok := <-evCh:
			if !ok {
				return nil
			}
			for _, answer := range ev.Answers {
				name := string(answer.Name)
				switch answer.Type { //nolint:exhaustive
				case layers.DNSTypeA:
					if cname, found := d.cnameToName.Get(name); found {
						name = cname
					}
					ip := answer.IP.To4()
					d.ipToName.Set(ip.String(), name, cache.WithExpiration(defaultDNSTTL))
				case layers.DNSTypeCNAME:
					cname := string(answer.CNAME)
					d.cnameToName.Set(cname, name, cache.WithExpiration(defaultDNSTTL))
				default:
					continue
				}
			}
		}
	}
}

func (d *IP2DNS) Lookup(ip netaddr.IP) string {
	value, _ := d.ipToName.Get(ip.String())
	return value
}
