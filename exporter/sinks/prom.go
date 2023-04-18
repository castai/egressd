package sinks

import (
	"context"
	"strconv"
	"time"

	"github.com/castai/promwrite"
	"github.com/sirupsen/logrus"
	"inet.af/netaddr"

	"github.com/castai/egressd/exporter/config"
	"github.com/castai/egressd/pb"
)

type promWriter interface {
	Write(ctx context.Context, req *promwrite.WriteRequest, options ...promwrite.WriteOption) (*promwrite.WriteResponse, error)
}

func NewPromRemoteWriteSink(log logrus.FieldLogger, sinkName string, cfg config.SinkPromRemoteWriteConfig) Sink {
	return &PromRemoteWriteSink{
		log: log.WithFields(map[string]interface{}{
			"sink_type": "prom_remote_write",
			"sink_name": sinkName,
		}),
		client:     promwrite.NewClient(cfg.URL),
		timeGetter: timeGetter,
	}
}

func timeGetter() time.Time {
	return time.Now().UTC()
}

type PromRemoteWriteSink struct {
	cfg        config.SinkPromRemoteWriteConfig
	log        logrus.FieldLogger
	client     promWriter
	timeGetter func() time.Time
}

func (s *PromRemoteWriteSink) Push(ctx context.Context, batch *pb.PodNetworkMetricBatch) error {
	var ts []promwrite.TimeSeries
	now := s.timeGetter()

	for _, m := range batch.Items {
		dstIP := ipFromInt32(m.DstIp)
		dstIPType := "public"
		if dstIP.IsPrivate() {
			dstIPType = "private"
		}
		ts = append(ts, promwrite.TimeSeries{
			Labels: []promwrite.Label{
				{Name: "__name__", Value: "egressd_transmit_bytes_total"},
				{Name: "src_pod", Value: m.SrcPod},
				{Name: "src_node", Value: m.SrcNode},
				{Name: "src_namespace", Value: m.SrcNamespace},
				{Name: "src_zone", Value: m.SrcZone},
				{Name: "src_ip", Value: ipFromInt32(m.SrcIp).String()},

				{Name: "dst_pod", Value: m.DstPod},
				{Name: "dst_node", Value: m.DstNode},
				{Name: "dst_namespace", Value: m.DstNamespace},
				{Name: "dst_zone", Value: m.DstZone},
				{Name: "dst_ip", Value: dstIP.String()},
				{Name: "dst_ip_type", Value: dstIPType},
				{Name: "cross_zone", Value: isCrossZoneValue(m)},

				{Name: "proto", Value: protoString(uint8(m.Proto))},
			},
			Sample: promwrite.Sample{
				Time:  now,
				Value: float64(m.TxBytes),
			},
		})
	}

	s.log.Infof("pushing metrics, timeseries=%d", len(ts))

	_, err := s.client.Write(ctx, &promwrite.WriteRequest{
		TimeSeries: ts,
	}, promwrite.WriteHeaders(s.cfg.Headers))
	return err
}

func isCrossZoneValue(m *pb.PodNetworkMetric) string {
	if m.SrcZone != "" && m.DstZone != "" && m.SrcZone != m.DstZone {
		return "true"
	}
	return "false"
}

func ipFromInt32(v int32) netaddr.IP {
	return netaddr.IPFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

var protoNames = map[uint8]string{
	0:  "ANY",
	1:  "ICMP",
	6:  "TCP",
	17: "UDP",
	58: "ICMPv6",
}

func protoString(p uint8) string {
	if _, ok := protoNames[p]; ok {
		return protoNames[p]
	}
	return strconv.Itoa(int(p))
}
