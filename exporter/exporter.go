package exporter

import (
	"context"

	"github.com/castai/egressd/collector"
	"github.com/castai/egressd/metrics"
	"github.com/cilium/lumberjack/v2"
	jsoniter "github.com/json-iterator/go"
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

	encoder := jsoniter.NewEncoder(writer)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case metric := <-e.metrics.GetMetricsChan():
			metrics.IncExportedEvents()
			if err := encoder.Encode(metric); err != nil {
				e.log.Errorf("writing metric to logs: %v", err)
			}
		}
	}
}
