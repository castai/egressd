package dns

import (
	"context"
	"net"
	"testing"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"

	"github.com/castai/egressd/ebpf"
	"github.com/castai/egressd/pb"
)

func TestIP2DNS_Records(t *testing.T) {
	type fields struct {
		Tracer tracer
	}

	tests := []struct {
		name   string
		fields fields
		want   []*pb.IP2Domain
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
			want: []*pb.IP2Domain{
				{Ip: "1.2.3.4", Domain: "example.com"},
				{Ip: "1.2.3.5", Domain: "hi.example.com"},
				{Ip: "1.2.3.6", Domain: "ehlo.example.com"},
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
			want: []*pb.IP2Domain{
				{Ip: "1.2.3.4", Domain: "example.com"},
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
			err := ip2dns.Start(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.want, ip2dns.Records())
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
