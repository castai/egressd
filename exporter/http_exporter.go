package exporter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/castai/egressd/collector"
	"github.com/castai/egressd/metrics"
	jsoniter "github.com/json-iterator/go"
	"github.com/sirupsen/logrus"
)

type HTTPConfig struct {
	Addr string
}

func NewHTTPExporter(cfg HTTPConfig, log logrus.FieldLogger, metrics metricsChanGetter) *HTTPExporter {
	return &HTTPExporter{
		cfg:     cfg,
		log:     log,
		metrics: metrics,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

type HTTPExporter struct {
	cfg     HTTPConfig
	log     logrus.FieldLogger
	metrics metricsChanGetter
	client  *http.Client
}

func (e *HTTPExporter) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case metric := <-e.metrics.GetMetricsChan():
			metrics.IncExportedEvents()
			if err := e.sendMetric(&metric); err != nil {
				e.log.Errorf("writing metric to http server: %v", err)
			}
		}
	}
}

func (e *HTTPExporter) sendMetric(m *collector.PodNetworkMetric) error {
	jsonBytes, err := jsoniter.ConfigFastest.Marshal(m)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", e.cfg.Addr, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("expected export http status 200, got %d, err: %s", resp.StatusCode, string(msg))
	}
	metrics.IncExportedEvents()
	return nil
}
