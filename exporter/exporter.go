package exporter

import (
	"context"
	"time"

	"github.com/cilium/lumberjack/v2"
	jsoniter "github.com/json-iterator/go"
	"github.com/sirupsen/logrus"

	"github.com/castai/egressd/collector"
	"github.com/castai/egressd/flusher"
	"github.com/castai/egressd/metrics"
)

type metricsChanGetter interface {
	GetMetricsChan() <-chan collector.PodNetworkMetric
}

type Config struct {
	FlushInterval       time.Duration
	ExportFilename      string
	ExportFileMaxSizeMB int
	MaxBackups          int
	Compress            bool
}

func New(cfg Config, log logrus.FieldLogger, metrics metricsChanGetter) *Exporter {
	writer := &lumberjack.Logger{
		Filename:   cfg.ExportFilename,
		MaxSize:    cfg.ExportFileMaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		Compress:   cfg.Compress,
	}
	dump := flusher.NewConntrackDump(jsoniter.NewEncoder(writer))

	return &Exporter{
		cfg:     cfg,
		log:     log,
		metrics: metrics,
		dump:    dump,
	}
}

type Exporter struct {
	cfg     Config
	log     logrus.FieldLogger
	metrics metricsChanGetter

	dump *flusher.ConntrackDump
}

func (e *Exporter) Start(ctx context.Context) error {
	ticker := time.NewTicker(e.cfg.FlushInterval)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case metric := <-e.metrics.GetMetricsChan():
			metrics.IncExportedEvents()
			e.dump.Dump(metric)
		case <-ticker.C:
			cnt, err := e.dump.Flush()
			if err != nil {
				e.log.Errorf("flushing metrics: %v", err)
			} else {
				e.log.Infof("flushed %d metrics", cnt)
			}
		}
	}
}
