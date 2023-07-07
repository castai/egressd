package dns

import (
	"context"

	"inet.af/netaddr"
)

type Noop struct{}

var _ LookuperStarter = (*Noop)(nil)

func (t *Noop) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
func (t *Noop) Lookup(ip netaddr.IP) string {
	return ""
}
