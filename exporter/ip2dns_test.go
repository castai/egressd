package exporter

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"inet.af/netaddr"

	"github.com/castai/egressd/pb"
)

func TestIP2DNS_Lookup(t *testing.T) {
	log := logrus.New()

	fields := []*pb.IP2Domain{
		{Ip: "1.2.3.4", Domain: "example.com"},
		{Ip: "1.2.3.5", Domain: "hi.example.com"},
		{Ip: "1.2.3.6", Domain: "ehlo.example.com"},
	}
	tests := []struct {
		name string
		wantIp string
		wantDomain string
	}{
		{
			name: "lookup existing IP",
			wantIp: "1.2.3.4",
			wantDomain: "example.com",
		},
		{
			name: "lookup not existing IP",
			wantIp: "4.3.2.1",
			wantDomain: "",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dns := newDnsStorage(ctx, log)
			dns.Fill(fields)

			ip := netaddr.MustParseIP(tt.wantIp)
			got := dns.Lookup(ip)
			require.Equal(t, tt.wantDomain, got, "Lookup(%s)", ip)
		})
	}
}
