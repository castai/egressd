package sinks

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"inet.af/netaddr"

	"github.com/castai/egressd/exporter/config"
	"github.com/castai/egressd/pb"
	"github.com/castai/promwrite"
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
		cfg:        cfg,
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
	now := s.timeGetter()

	ts := make([]promwrite.TimeSeries, 0, len(batch.Items))

	for _, m := range batch.Items {
		dstIP, _ := netaddr.ParseIP(m.DstIp)
		dstIPType := "public"
		if dstIP.IsPrivate() {
			dstIPType = "private"
		}
		// Initial labels, sorted by label name asc.
		labels := []promwrite.Label{
			{Name: "__name__", Value: "egressd_transmit_bytes_total"},
			{Name: "cross_zone", Value: isCrossZoneValue(m)},
			{Name: "dst_ip", Value: dstIP.String()},
			{Name: "dst_ip_type", Value: dstIPType},
			{Name: "dst_namespace", Value: m.DstNamespace},
			{Name: "dst_node", Value: m.DstNode},
			{Name: "dst_pod", Value: m.DstPod},
			{Name: "dst_zone", Value: m.DstZone},
			{Name: "proto", Value: protoString(uint8(m.Proto))},
			{Name: "src_ip", Value: m.SrcIp},
			{Name: "src_namespace", Value: m.SrcNamespace},
			{Name: "src_node", Value: m.SrcNode},
			{Name: "src_pod", Value: m.SrcPod},
			{Name: "src_zone", Value: m.SrcZone},
		}

		if customLabelsCount := len(s.cfg.Labels); customLabelsCount > 0 {
			labels = slices.Grow(labels, customLabelsCount)
			for k, v := range s.cfg.Labels {
				labels = append(labels, promwrite.Label{Name: k, Value: v})
			}
			// Some prometheus like databases requires sorted labels. See https://github.com/castai/egressd/pull/109
			slices.SortStableFunc(labels, func(a, b promwrite.Label) int {
				return strings.Compare(a.Name, b.Name)
			})
		}

		ts = append(ts, promwrite.TimeSeries{
			Labels: labels,
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
