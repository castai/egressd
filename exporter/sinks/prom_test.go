package sinks

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/castai/promwrite"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/castai/egressd/exporter/config"
	"github.com/castai/egressd/pb"
)

func TestPromSink(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

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

	expectedLabels := []promwrite.Label{
		{Name: "__name__", Value: "egressd_transmit_bytes_total"},
		{Name: "src_pod", Value: "p1"},
		{Name: "src_node", Value: "n1"},
		{Name: "src_namespace", Value: "team1"},
		{Name: "src_zone", Value: "us-east-1a"},
		{Name: "src_ip", Value: "10.14.7.12"},
		{Name: "dst_pod", Value: "p2"},
		{Name: "dst_node", Value: "n1"},
		{Name: "dst_namespace", Value: "team2"},
		{Name: "dst_zone", Value: "us-east-1a"},
		{Name: "dst_ip", Value: "10.14.7.5"},
		{Name: "dst_ip_type", Value: "private"},
		{Name: "cross_zone", Value: "false"},
		{Name: "proto", Value: "TCP"},
	}
	sort.Slice(expectedLabels, func(i, j int) bool {
		return expectedLabels[i].Name < expectedLabels[j].Name
	})

	r.Equal([]promwrite.TimeSeries{
		{
			Labels: expectedLabels,
			Sample: promwrite.Sample{Time: ts, Value: 35},
		},
	}, client.req.TimeSeries)
}

type mockPromWriteClient struct {
	req *promwrite.WriteRequest
}

func (m *mockPromWriteClient) Write(ctx context.Context, req *promwrite.WriteRequest, options ...promwrite.WriteOption) (*promwrite.WriteResponse, error) {
	m.req = req
	return nil, nil
}
