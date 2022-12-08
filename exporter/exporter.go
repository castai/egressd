package exporter

import (
	"bytes"
	"context"
	"strconv"
	"sync"

	"github.com/castai/egressd/collector"
	"github.com/castai/egressd/metrics"
	"github.com/cilium/lumberjack/v2"
	"github.com/sirupsen/logrus"
)

type metricsChanGetter interface {
	GetMetricsChan() <-chan collector.PodNetworkMetric
}

type Config struct {
	ExportFilename      string
	ExportFileMaxSizeMB int
	MaxBackups          int
	Compress            bool
}

func New(cfg Config, log logrus.FieldLogger, metrics metricsChanGetter) *Exporter {
	return &Exporter{
		cfg:     cfg,
		log:     log,
		metrics: metrics,
	}
}

type Exporter struct {
	cfg     Config
	log     logrus.FieldLogger
	metrics metricsChanGetter
}

func (e *Exporter) Start(ctx context.Context) error {
	writer := &lumberjack.Logger{
		Filename:   e.cfg.ExportFilename,
		MaxSize:    e.cfg.ExportFileMaxSizeMB,
		MaxBackups: e.cfg.MaxBackups,
		Compress:   e.cfg.Compress,
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case metric := <-e.metrics.GetMetricsChan():
			metrics.IncExportedEvents()
			if _, err := writer.Write(marshalJSON(&metric)); err != nil {
				e.log.Errorf("writing metric to logs: %v", err)
			}
		}
	}
}

var pool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer([]byte{})
	},
}

func marshalJSON(m *collector.PodNetworkMetric) []byte {
	buf := pool.Get().(*bytes.Buffer)
	buf.Reset()

	buf.WriteByte('{')
	buf.WriteString(`"src_ip":"` + m.SrcIP + `"`)
	buf.WriteString(`,"src_pod":"` + m.SrcPod + `"`)
	buf.WriteString(`,"src_namespace":"` + m.SrcNamespace + `"`)
	buf.WriteString(`,"src_node":"` + m.SrcNode + `"`)
	buf.WriteString(`,"src_zone":"` + m.SrcZone + `"`)
	buf.WriteString(`,"dst_ip":"` + m.DstIP + `"`)
	buf.WriteString(`,"dst_ip_type":"` + m.DstIPType + `"`)
	if m.DstPod != "" {
		buf.WriteString(`,"dst_pod":"` + m.DstPod + `"`)
		buf.WriteString(`,"dst_namespace":"` + m.DstNamespace + `"`)
		buf.WriteString(`,"dst_node":"` + m.DstNode + `"`)
		buf.WriteString(`,"dst_zone":"` + m.DstZone + `"`)
	}
	buf.WriteString(`,"tx_bytes":` + strconv.Itoa(int(m.TxBytes)))
	buf.WriteString(`,"tx_packets":` + strconv.Itoa(int(m.TxPackets)))
	buf.WriteString(`,"rx_bytes":` + strconv.Itoa(int(m.RxBytes)))
	buf.WriteString(`,"rx_packets":` + strconv.Itoa(int(m.RxPackets)))
	buf.WriteString(`,"proto":"` + m.Proto + `"`)
	buf.WriteString(`,"ts":` + strconv.Itoa(int(m.TS)))
	buf.WriteByte('}')
	buf.WriteByte('\n')

	pool.Put(buf)
	return buf.Bytes()
}
