package exporter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/castai/egressd/collector"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestHTTPExporter(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	metrics := &metricsGetter{
		ch: make(chan *collector.PodNetworkMetric, 10),
	}

	receivedMetrics := make(chan collector.PodNetworkMetric)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var metric collector.PodNetworkMetric
		if err := json.NewDecoder(r.Body).Decode(&metric); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		receivedMetrics <- metric
		w.WriteHeader(http.StatusOK)
	}))

	exp := NewHTTPExporter(HTTPConfig{Addr: srv.URL}, log, metrics)
	startErr := make(chan error)
	go func() {
		if err := exp.Start(ctx); err != nil {
			startErr <- err
		}
	}()

	testMetric := &collector.PodNetworkMetric{
		SrcIP:        "1",
		SrcPod:       "2",
		SrcNamespace: "3",
		SrcNode:      "4",
		SrcZone:      "5",
		DstIP:        "6",
		DstIPType:    "7",
		DstPod:       "8",
		DstNamespace: "9",
		DstNode:      "10",
		DstZone:      "11",
		TxBytes:      12,
		TxPackets:    13,
		RxBytes:      14,
		RxPackets:    015,
		Proto:        "16",
		TS:           17,
	}
	metrics.ch <- testMetric

	timeout := time.After(3 * time.Second)
	select {
	case m := <-receivedMetrics:
		r.Equal(*testMetric, m)
	case err := <-startErr:
		t.Fatal(err)
	case <-timeout:
		t.Fatal("timeout waiting for metric")
	}
}

type metricsGetter struct {
	ch chan *collector.PodNetworkMetric
}

func (m *metricsGetter) GetMetricsChan() <-chan *collector.PodNetworkMetric {
	return m.ch
}
