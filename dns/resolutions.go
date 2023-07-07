package dns

import (
	"context"
	"sync"
	"time"

	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/google/gopacket/layers"
	"inet.af/netaddr"

	"github.com/castai/egressd/ebpf"
)

type LookuperStarter interface {
	Start(ctx context.Context) error
	Lookup(ip netaddr.IP) string
}

var _ LookuperStarter = (*IP2DNS)(nil)

type tracer interface {
	Run(ctx context.Context) error
	Events() <-chan ebpf.DNSEvent
}

var defaultDNSTTL = 5 * time.Minute

type IP2DNS struct {
	Tracer      tracer
	ipToName    *cache.Cache[string, string]
	cnameToName *cache.Cache[string, string]
	mu          sync.RWMutex
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
	//<-time.After(1 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errch:
			if err != nil {
				return err
			}
			return nil
		case ev, ok := <-evCh:
			if !ok {
				return nil
			}
			func() {
				d.mu.Lock()
				defer d.mu.Unlock()
				for _, answer := range ev.Answers {
					name := string(answer.Name)
					ttl := time.Duration(answer.TTL) * time.Second
					if ttl == 0 {
						ttl = defaultDNSTTL
					}
					switch answer.Type { //nolint:exhaustive
					case layers.DNSTypeA:
						if cname, found := d.cnameToName.Get(name); found {
							name = cname
						}
						ip := answer.IP.To4()
						d.ipToName.Set(ip.String(), name, cache.WithExpiration(ttl))
					case layers.DNSTypeCNAME:
						cname := string(answer.CNAME)
						d.cnameToName.Set(cname, name, cache.WithExpiration(ttl))
					default:
						continue
					}
				}
			}()
		}
	}
}

func (d *IP2DNS) Lookup(ip netaddr.IP) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	value, _ := d.ipToName.Get(ip.String())
	return value
}
