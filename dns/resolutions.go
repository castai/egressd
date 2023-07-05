package dns

import (
	"context"
	"sync"

	"github.com/google/gopacket/layers"
	"inet.af/netaddr"

	"github.com/castai/egressd/ebpf"
)

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

func (x *IP2DNS) Start(ctx context.Context) error {
	x.ipToName = make(map[string]string)
	x.cnameToName = make(map[string]string)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errch := make(chan error, 1)
	go func() {
		err := x.Tracer.Run(ctx)
		errch <- err
	}()

	evCh := x.Tracer.Events()
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
				x.mu.Lock()
				defer x.mu.Unlock()
				for _, answer := range ev.Answers {
					name := string(answer.Name)
					switch answer.Type {
					case layers.DNSTypeA:
						if cname, found := x.cnameToName[name]; found {
							name = cname
						}
						ip := answer.IP.To4()
						x.ipToName[ip.String()] = name
					case layers.DNSTypeCNAME:
						cname := string(answer.CNAME)
						x.cnameToName[cname] = name
					}
				}
			}()
		}
	}
}

func (x *IP2DNS) Lookup(ip netaddr.IP) string {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.ipToName[ip.String()]
}
