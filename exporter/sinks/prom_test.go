package sinks

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/castai/promwrite"

	"github.com/castai/egressd/exporter/config"
	"github.com/castai/egressd/pb"
)

func TestPromSink(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	t.Run("push", func(t *testing.T) {
		cfg := config.SinkPromRemoteWriteConfig{
			URL: "prom-remote-write-url",
			Headers: map[string]string{
				"Custom-Header": "1",
			},
		}
		client := &mockPromWriteClient{}
		ts := time.Date(2023, time.April, 13, 13, 41, 48, 926278000, time.UTC)
		sink := PromRemoteWriteSink{
			cfg:    cfg,
			log:    log,
			client: client,
			timeGetter: func() time.Time {
				return ts
			},
		}
		batch := &pb.PodNetworkMetricBatch{
			Items: []*pb.PodNetworkMetric{
				{
					SrcIp:        "10.14.7.12",
					SrcPod:       "p1",
					SrcNamespace: "team1",
					SrcNode:      "n1",
					SrcZone:      "us-east-1a",
					DstIp:        "10.14.7.5",
					DstPod:       "p2",
					DstNamespace: "team2",
					DstNode:      "n1",
					DstZone:      "us-east-1a",
					TxBytes:      35,
					TxPackets:    3,
					RxBytes:      30,
					RxPackets:    1,
					Proto:        6,
				},
			},
		}
		r.NoError(sink.Push(ctx, batch))

		r.Equal([]promwrite.TimeSeries{
			{
				Labels: []promwrite.Label{
					{Name: "__name__", Value: "egressd_transmit_bytes_total"},
					{Name: "cross_zone", Value: "false"},
					{Name: "dst_ip", Value: "10.14.7.5"},
					{Name: "dst_ip_type", Value: "private"},
					{Name: "dst_namespace", Value: "team2"},
					{Name: "dst_node", Value: "n1"},
					{Name: "dst_pod", Value: "p2"},
					{Name: "dst_zone", Value: "us-east-1a"},
					{Name: "proto", Value: "TCP"},
					{Name: "src_ip", Value: "10.14.7.12"},
					{Name: "src_namespace", Value: "team1"},
					{Name: "src_node", Value: "n1"},
					{Name: "src_pod", Value: "p1"},
					{Name: "src_zone", Value: "us-east-1a"},
				},
				Sample: promwrite.Sample{Time: ts, Value: 35},
			},
		},
			client.reqs[0].TimeSeries)
	})

	t.Run("push with custom labels", func(t *testing.T) {
		cfg := config.SinkPromRemoteWriteConfig{
			URL: "prom-remote-write-url",
			Headers: map[string]string{
				"Custom-Header": "1",
			},
			Labels: map[string]string{
				"xz_label_key": "xz_label_value",
			},
			SendReceivedBytesMetric: true,
		}
		client := &mockPromWriteClient{}
		ts := time.Date(2023, time.April, 13, 13, 41, 48, 926278000, time.UTC)
		sink := PromRemoteWriteSink{
			cfg:    cfg,
			log:    log,
			client: client,
			timeGetter: func() time.Time {
				return ts
			},
		}
		batch := &pb.PodNetworkMetricBatch{
			Items: []*pb.PodNetworkMetric{
				{
					SrcIp:        "10.14.7.12",
					SrcPod:       "p1",
					SrcNamespace: "team1",
					SrcNode:      "n1",
					SrcZone:      "us-east-1a",
					DstIp:        "10.14.7.5",
					DstPod:       "p2",
					DstNamespace: "team2",
					DstNode:      "n1",
					DstZone:      "us-east-1a",
					TxBytes:      35,
					TxPackets:    3,
					RxBytes:      30,
					RxPackets:    1,
					Proto:        6,
				},
			},
		}
		r.NoError(sink.Push(ctx, batch))
		r.Len(client.reqs, 2)

		slices.SortFunc(client.reqs, func(a, b *promwrite.WriteRequest) int {
			return strings.Compare(a.TimeSeries[0].Labels[0].Name, b.TimeSeries[0].Labels[0].Name)
		})

		r.Equal([]promwrite.TimeSeries{
			{
				Labels: []promwrite.Label{
					{Name: "__name__", Value: "egressd_received_bytes_total"},
					{Name: "cross_zone", Value: "false"},
					{Name: "dst_ip", Value: "10.14.7.5"},
					{Name: "dst_ip_type", Value: "private"},
					{Name: "dst_namespace", Value: "team2"},
					{Name: "dst_node", Value: "n1"},
					{Name: "dst_pod", Value: "p2"},
					{Name: "dst_zone", Value: "us-east-1a"},
					{Name: "proto", Value: "TCP"},
					{Name: "src_ip", Value: "10.14.7.12"},
					{Name: "src_namespace", Value: "team1"},
					{Name: "src_node", Value: "n1"},
					{Name: "src_pod", Value: "p1"},
					{Name: "src_zone", Value: "us-east-1a"},
					{Name: "xz_label_key", Value: "xz_label_value"},
				},
				Sample: promwrite.Sample{Time: ts, Value: 30},
			},
		}, client.reqs[0].TimeSeries)

		r.Equal([]promwrite.TimeSeries{
			{
				Labels: []promwrite.Label{
					{Name: "__name__", Value: "egressd_transmit_bytes_total"},
					{Name: "cross_zone", Value: "false"},
					{Name: "dst_ip", Value: "10.14.7.5"},
					{Name: "dst_ip_type", Value: "private"},
					{Name: "dst_namespace", Value: "team2"},
					{Name: "dst_node", Value: "n1"},
					{Name: "dst_pod", Value: "p2"},
					{Name: "dst_zone", Value: "us-east-1a"},
					{Name: "proto", Value: "TCP"},
					{Name: "src_ip", Value: "10.14.7.12"},
					{Name: "src_namespace", Value: "team1"},
					{Name: "src_node", Value: "n1"},
					{Name: "src_pod", Value: "p1"},
					{Name: "src_zone", Value: "us-east-1a"},
					{Name: "xz_label_key", Value: "xz_label_value"},
				},
				Sample: promwrite.Sample{Time: ts, Value: 35},
			},
		}, client.reqs[1].TimeSeries)
	})

}

type mockPromWriteClient struct {
	reqs []*promwrite.WriteRequest
	m    sync.Mutex
}

func (m *mockPromWriteClient) Write(ctx context.Context, req *promwrite.WriteRequest, options ...promwrite.WriteOption) (*promwrite.WriteResponse, error) {
	m.m.Lock()
	m.reqs = append(m.reqs, req)
	m.m.Unlock()
	return nil, nil
}
