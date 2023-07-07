package dns

import (
	"context"
	"net"
	"testing"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"
	"inet.af/netaddr"

	"github.com/castai/egressd/ebpf"
)

func TestIP2DNS_Lookup(t *testing.T) {
	type fields struct {
		Tracer tracer
	}

	tests := []struct {
		name   string
		fields fields
		want   map[string]string
	}{
		{
			name: "resolves simple A record",
			fields: fields{tracerWithEvents([]ebpf.DNSEvent{
				{
					Questions: nil,
					Answers: []layers.DNSResourceRecord{
						{
							Name: []byte("example.com"),
							Type: layers.DNSTypeA,
							IP:   net.IPv4(1, 2, 3, 4),
						},
						{
							Name: []byte("hi.example.com"),
							Type: layers.DNSTypeA,
							IP:   net.IPv4(1, 2, 3, 5),
						},
						{
							Name: []byte("ehlo.example.com"),
							Type: layers.DNSTypeA,
							IP:   net.IPv4(1, 2, 3, 6),
						},
					},
				},
			})},
			want: map[string]string{
				"1.2.3.4": "example.com",
				"1.2.3.5": "hi.example.com",
				"1.2.3.6": "ehlo.example.com",
				"4.3.2.1": "",
			},
		},

		{
			name: "resolves in-order CNAME record",
			fields: fields{tracerWithEvents([]ebpf.DNSEvent{
				{
					Questions: nil,
					Answers: []layers.DNSResourceRecord{
						{
							Name:  []byte("example.com"),
							Type:  layers.DNSTypeCNAME,
							CNAME: []byte("cname.example.com"),
						},
						{
							Name: []byte("cname.example.com"),
							Type: layers.DNSTypeA,
							IP:   net.IPv4(1, 2, 3, 4),
						},
					},
				},
			})},
			want: map[string]string{
				"1.2.3.4": "example.com",
				"4.3.2.1": "",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ip2dns := &IP2DNS{
				Tracer: tt.fields.Tracer,
			}
			_ = ip2dns.Start(ctx)
			for arg, want := range tt.want {
				ip := netaddr.MustParseIP(arg)
				got := ip2dns.Lookup(ip)
				require.Equal(t, want, got, "Lookup(%s)", ip)
			}
		})
	}
}

func tracerWithEvents(events []ebpf.DNSEvent) *mockTracer {
	ch := make(chan ebpf.DNSEvent)
	return &mockTracer{
		events:    events,
		eventChan: ch,
	}
}

type mockTracer struct {
	eventChan chan ebpf.DNSEvent
	events    []ebpf.DNSEvent
}

func (m *mockTracer) Run(ctx context.Context) error {
	for _, ev := range m.events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m.eventChan <- ev:
		}
	}
	close(m.eventChan)
	return nil
}

func (m *mockTracer) Events() <-chan ebpf.DNSEvent {
	return m.eventChan
}
