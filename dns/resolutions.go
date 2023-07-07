package dns

import (
	"context"
	"sync"

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

type IP2DNS struct {
	Tracer      tracer
	ipToName    map[string]string
	cnameToName map[string]string
	mu          sync.RWMutex
}

func (d *IP2DNS) Start(ctx context.Context) error {
	d.ipToName = make(map[string]string)
	d.cnameToName = make(map[string]string)

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
					switch answer.Type {
					case layers.DNSTypeA:
						if cname, found := d.cnameToName[name]; found {
							name = cname
						}
						ip := answer.IP.To4()
						d.ipToName[ip.String()] = name
					case layers.DNSTypeCNAME:
						cname := string(answer.CNAME)
						d.cnameToName[cname] = name
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
	return d.ipToName[ip.String()]
}
