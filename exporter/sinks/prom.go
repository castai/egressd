package sinks

import (
	"context"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"inet.af/netaddr"

	"github.com/castai/egressd/exporter/config"
	"github.com/castai/egressd/pb"
	"github.com/castai/promwrite"
	"fmt"
	"strings"
)

type promWriter interface {
	Write(ctx context.Context, req *promwrite.WriteRequest, options ...promwrite.WriteOption) (*promwrite.WriteResponse, error)
}

func NewPromRemoteWriteSink(log logrus.FieldLogger, sinkName string, cfg config.SinkPromRemoteWriteConfig, extraLabels ExtraLabels) Sink {
	return &PromRemoteWriteSink{
		log: log.WithFields(map[string]interface{}{
			"sink_type": "prom_remote_write",
			"sink_name": sinkName,
		}),
		client:      promwrite.NewClient(cfg.URL),
		timeGetter:  timeGetter,
		cfg:         cfg,
		extraLabels: extraLabels,
	}
}

func timeGetter() time.Time {
	return time.Now().UTC()
}

type PromRemoteWriteSink struct {
	cfg         config.SinkPromRemoteWriteConfig
	log         logrus.FieldLogger
	client      promWriter
	timeGetter  func() time.Time
	extraLabels ExtraLabels
}

func (s *PromRemoteWriteSink) Push(ctx context.Context, batch *pb.PodNetworkMetricBatch) error {
	var ts []promwrite.TimeSeries
	now := s.timeGetter()

	for _, m := range batch.Items {
		dstIP, _ := netaddr.ParseIP(m.DstIp)
		dstIPType := "public"
		if dstIP.IsPrivate() {
			dstIPType = "private"
		}
		labels := []promwrite.Label{
			{Name: "__name__", Value: "egressd_transmit_bytes_total"},
			{Name: "src_pod", Value: m.SrcPod},
			{Name: "src_node", Value: m.SrcNode},
			{Name: "src_namespace", Value: m.SrcNamespace},
			{Name: "src_zone", Value: m.SrcZone},
			{Name: "src_ip", Value: m.SrcIp},

			{Name: "dst_pod", Value: m.DstPod},
			{Name: "dst_node", Value: m.DstNode},
			{Name: "dst_namespace", Value: m.DstNamespace},
			{Name: "dst_zone", Value: m.DstZone},
			{Name: "dst_ip", Value: dstIP.String()},
			{Name: "dst_ip_type", Value: dstIPType},
			{Name: "cross_zone", Value: isCrossZoneValue(m)},

			{Name: "proto", Value: protoString(uint8(m.Proto))},
		}
		// add any defined extra labels
		for k, v := range s.extraLabels {
			labels = append(labels, promwrite.Label{Name: k, Value: v})
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

// ExtraLabels is a custom type that implements the flag.Value interface
type ExtraLabels map[string]string

// String is part of the flag.Value interface
func (l *ExtraLabels) String() string {
	return fmt.Sprint(*l)
}

// Set is part of the flag.Value interface
// It parses a key=value pair and adds it to the map
func (l *ExtraLabels) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid label format: %s", value)
	}
	key := parts[0]
	val := parts[1]
	(*l)[key] = val
	return nil
}
