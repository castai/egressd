package exporter

import (
	"context"
	"time"

	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/castai/egressd/pb"
	"github.com/sirupsen/logrus"
	"inet.af/netaddr"
)

var defaultDNSTTL = 1 * time.Hour

type ip2dns struct {
	log      logrus.FieldLogger
	ipToName *cache.Cache[string, string]
}

func newDNSCache(ctx context.Context, log logrus.FieldLogger) *ip2dns {
	return &ip2dns{
		log:    log,
		ipToName: cache.NewContext[string, string](ctx),
	}
}

func (d *ip2dns) Fill(items []*pb.IP2Domain) {
	for i := range items {
		d.ipToName.Set(items[i].Ip, items[i].Domain, cache.WithExpiration(defaultDNSTTL))
	}
}

func (d *ip2dns) Lookup(ip netaddr.IP) string {
	value, ok := d.ipToName.Get(ip.String())
	if !ok {
		d.log.Debugf("domain not found for IP %q", ip.String())
	}
	return value
}
