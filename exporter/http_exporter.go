package exporter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/castai/egressd/collector"
	"github.com/castai/egressd/metrics"
	jsoniter "github.com/json-iterator/go"
	"github.com/sirupsen/logrus"
)

var retryBackoff = []time.Duration{
	10 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
}

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
			if err := e.sendMetric(ctx, metric); err != nil && !errors.Is(err, context.Canceled) {
				e.log.Errorf("writing metric to http server: %v", err)
			}
		}
	}
}

func (e *HTTPExporter) sendMetric(ctx context.Context, m *collector.PodNetworkMetric) error {
	var buf bytes.Buffer
	if err := jsoniter.ConfigFastest.NewEncoder(&buf).Encode(m); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", e.cfg.Addr, &buf)
	if err != nil {
		return err
	}

	send := func() error {
		resp, err := e.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			msg, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("expected export http status 200, got %d, err: %s", resp.StatusCode, string(msg))
		}
		return nil
	}

	backoff := func(fn func() error) error {
		var err error
		for _, b := range retryBackoff {
			err = fn()
			if err == nil {
				return nil
			}
			e.log.Warnf("failed to send metrics, sleeping %s: %v", b, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(b):
			}
		}
		return err
	}

	if err := backoff(send); err != nil {
		return err
	}

	metrics.IncExportedEvents()
	return nil
}
