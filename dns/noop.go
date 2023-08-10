package dns

import (
	"context"
	"net/http"

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

func (d *Noop) GetIp2DnsHandler(w http.ResponseWriter, req *http.Request) {
	var batchBytes []byte
	w.WriteHeader(http.StatusNoContent)
	if _, err := w.Write(batchBytes); err != nil {
		return
	}
}
