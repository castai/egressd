package dns

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/google/gopacket/layers"
	"github.com/sirupsen/logrus"
	"inet.af/netaddr"

	"github.com/castai/egressd/ebpf"
	"github.com/castai/egressd/pb"
)

type DNSCollector interface {
	Start(ctx context.Context) error
	Records() []*pb.IP2Domain
}

var _ DNSCollector = (*IP2DNS)(nil)

type tracer interface {
	Run(ctx context.Context) error
	Events() <-chan ebpf.DNSEvent
}

var defaultDNSTTL = 2 * time.Minute

type IP2DNS struct {
	Tracer      tracer
	log         logrus.FieldLogger
	ipToName    *cache.Cache[int32, string]
	cnameToName *cache.Cache[string, string]
}

func NewIP2DNS(tracer tracer, log logrus.FieldLogger) *IP2DNS {
	return &IP2DNS{
		Tracer: tracer,
		log:    log,
	}
}

func (d *IP2DNS) Start(ctx context.Context) error {

	d.ipToName = cache.NewContext[int32, string](ctx)
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
					ip, _ := netaddr.FromStdIP(answer.IP)
					d.ipToName.Set(ToIPint32(ip), name, cache.WithExpiration(defaultDNSTTL))
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

func (d *IP2DNS) Records() []*pb.IP2Domain {
	items := make([]*pb.IP2Domain, 0, len(d.ipToName.Keys()))
	for _, ip := range d.ipToName.Keys() {
		domain, _ := d.ipToName.Get(ip)
		items = append(items, &pb.IP2Domain{Ip: ip, Domain: domain})
	}
	return items
}

func ToIPint32(ip netaddr.IP) int32 {
	b := ip.As4()
	return int32(binary.BigEndian.Uint32([]byte{b[0], b[1], b[2], b[3]}))
}
