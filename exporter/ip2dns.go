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

type dnsStorage struct {
	log      logrus.FieldLogger
	ipToName *cache.Cache[string, string]
}

func newDnsStorage(ctx context.Context, log logrus.FieldLogger) *dnsStorage {
	return &dnsStorage{
		log:    log,
		ipToName: cache.NewContext[string, string](ctx),
	}
}

func (d *dnsStorage) Fill(items []*pb.IP2Domain) {
	for i := range items {
		d.ipToName.Set(items[i].Ip, items[i].Domain, cache.WithExpiration(defaultDNSTTL))
	}
}

func (d *dnsStorage) Lookup(ip netaddr.IP) string {
	value, ok := d.ipToName.Get(ip.String())
	if !ok {
		d.log.Debugf("domain not found for IP %q", ip.String())
	}
	return value
}
