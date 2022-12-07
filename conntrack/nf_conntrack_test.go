package conntrack

import (
	"io"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"inet.af/netaddr"
)

func TestNetfilterConntrack(t *testing.T) {
	r := require.New(t)
	log := logrus.New()
	records := `ipv4     2 tcp      6 86391 ESTABLISHED src=127.0.0.1 dst=127.0.0.1 sport=36350 dport=10231 src=127.0.0.1 dst=127.0.0.1 sport=10231 dport=36350 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 74 TIME_WAIT src=169.254.169.254 dst=35.236.111.222 sport=34410 dport=10256 packets=6 bytes=423 src=35.236.111.222 dst=169.254.169.254 sport=10256 dport=34410 packets=4 bytes=507 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 76 TIME_WAIT src=10.20.1.1 dst=10.30.1.6 sport=49180 dport=19090 packets=5 bytes=377 src=10.20.1.6 dst=10.20.1.1 sport=19090 dport=49180 packets=5 bytes=425 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 13 TIME_WAIT src=10.20.1.2 dst=10.30.1.7 sport=49180 dport=19090 packets=5 bytes=377 src=10.20.1.5 dst=10.20.1.2 sport=19090 dport=49180 packets=5 bytes=425 [ASSURED] mark=0 zone=0 use=2
`
	client := NewNetfilterClient(log, func() (io.ReadCloser, error) {
		return &mockConntrackReader{
			records: strings.NewReader(records),
		}, nil
	})

	srcIPs := map[netaddr.IP]struct{}{
		netaddr.MustParseIP("127.0.0.1"):       {}, // Will not much since this flow doesn't have bytes stats.
		netaddr.MustParseIP("169.254.169.254"): {},
		netaddr.MustParseIP("10.20.1.1"):       {},
	}
	entries, err := client.ListEntries(EgressWithAccounting(srcIPs))
	r.NoError(err)
	r.Len(entries, 2)
	_, found := entries[netaddr.MustParseIP("169.254.169.254")]
	r.True(found)
	_, found = entries[netaddr.MustParseIP("10.20.1.1")]
	r.True(found)
}

func TestNetfilterLineParser(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected conntrackEntry
	}{
		{
			name: "parse without packets and bytes",
			line: "ipv4     2 tcp      6 86391 ESTABLISHED src=127.0.0.1 dst=127.0.0.1 sport=36350 dport=10231 src=127.0.0.1 dst=127.0.0.1 sport=10231 dport=36350 [ASSURED] mark=0 zone=0 use=2",
			expected: conntrackEntry{
				proto:     6,
				reqSrc:    netaddr.MustParseIPPort("127.0.0.1:36350"),
				reqDst:    netaddr.MustParseIPPort("127.0.0.1:10231"),
				respSrc:   netaddr.MustParseIPPort("127.0.0.1:10231"),
				respDst:   netaddr.MustParseIPPort("127.0.0.1:36350"),
				txBytes:   0,
				txPackets: 0,
				rxBytes:   0,
				rxPackets: 0,
			},
		},
		{
			name: "parse with packets and bytes",
			line: "ipv4     2 tcp      6 74 TIME_WAIT src=169.254.169.254 dst=35.236.111.222 sport=34410 dport=10256 packets=6 bytes=423 src=35.236.111.222 dst=169.254.169.254 sport=10256 dport=34410 packets=4 bytes=507 [ASSURED] mark=0 zone=0 use=2",
			expected: conntrackEntry{
				proto:     6,
				reqSrc:    netaddr.MustParseIPPort("169.254.169.254:34410"),
				reqDst:    netaddr.MustParseIPPort("35.236.111.222:10256"),
				respSrc:   netaddr.MustParseIPPort("35.236.111.222:10256"),
				respDst:   netaddr.MustParseIPPort("169.254.169.254:34410"),
				txPackets: 6,
				txBytes:   423,
				rxPackets: 4,
				rxBytes:   507,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := require.New(t)
			res := parseConntrackLine(tt.line)
			r.Equal(tt.expected, res)
		})
	}
}

// Profile with pprof:
// go test -bench=. -benchmem -memprofile mem.out -cpuprofile cpu.out  -run=BenchmarkNetfilterLineParser -gcflags -m=2
// go tool pprof -noinlines mem.out
func BenchmarkNetfilterLineParser(b *testing.B) {
	l := "ipv4     2 tcp      6 74 TIME_WAIT src=169.254.169.254 dst=35.236.111.222 sport=34410 dport=10256 packets=6 bytes=423 src=35.236.111.222 dst=169.254.169.254 sport=10256 dport=34410 packets=4 bytes=507 [ASSURED] mark=0 zone=0 use=2"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parseConntrackLine(l)
	}
}

type mockConntrackReader struct {
	records *strings.Reader
}

func (m *mockConntrackReader) Read(p []byte) (n int, err error) {
	return m.records.Read(p)
}

func (m *mockConntrackReader) Close() error {
	return nil
}
