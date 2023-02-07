package exporter

import (
	"context"

	"github.com/castai/egressd/collector"
)

type metricsChanGetter interface {
	GetMetricsChan() <-chan collector.PodNetworkMetric
}

type Exporter interface {
	Start(ctx context.Context) error
}
