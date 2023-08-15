package exporter

import (
	"context"
	"time"

	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/sirupsen/logrus"

	"github.com/castai/egressd/pb"
)

var defaultDNSTTL = 1 * time.Hour

type dnsStorage struct {
	log      logrus.FieldLogger
	ipToName *cache.Cache[int32, string]
}

func newDnsStorage(ctx context.Context, log logrus.FieldLogger) *dnsStorage {
	return &dnsStorage{
		log:      log,
		ipToName: cache.NewContext[int32, string](ctx),
	}
}

func (d *dnsStorage) Fill(items []*pb.IP2Domain) {
	for i := range items {
		d.ipToName.Set(items[i].Ip, items[i].Domain, cache.WithExpiration(defaultDNSTTL))
	}
}

func (d *dnsStorage) Lookup(ip int32) string {
	value, ok := d.ipToName.Get(ip)
	if !ok {
		d.log.Debugf("domain not found for IP %q", ipFromInt32(ip))
	}
	return value
}
