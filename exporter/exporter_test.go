package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/castai/egressd/collector"
	jsoniter "github.com/json-iterator/go"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestExporter(t *testing.T) {
	r := require.New(t)
	log := logrus.New()
	metrics := &mockMetricsChanGetter{
		ch: make(chan collector.PodNetworkMetric, 10),
	}
	fileName := filepath.Join(t.TempDir(), "egressd.log")
	exp := New(Config{
		ExportFilename:      fileName,
		ExportFileMaxSizeMB: 1,
		MaxBackups:          2,
		Compress:            false,
	}, log, metrics)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exporterErr := make(chan error)
	go func() {
		if err := exp.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			exporterErr <- err
		}
	}()

	metrics.ch <- newTestMetric()
	metrics.ch <- newTestMetric()

	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-time.After(100 * time.Millisecond):
			logs, err := os.ReadFile(fileName)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			r.NoError(err)
			lines := strings.Split(string(logs), "\n")
			if len(lines) == 3 {
				r.NotEmpty(lines[0])
				r.NotEmpty(lines[1])
				r.Equal("", lines[2])
				return
			}
		case <-timeout:
			t.Fatal("timeout waiting to logs file")
		}
	}
}

func TestPodNetworkMetricMarshal(t *testing.T) {
	r := require.New(t)
	metric := collector.PodNetworkMetric{
		SrcIP:        "src_ip",
		SrcPod:       "src_pod",
		SrcNamespace: "src_ns",
		SrcNode:      "src_node",
		SrcZone:      "src_zone",
		DstIP:        "dst_ip",
		DstIPType:    "private",
		DstPod:       "dst_pod",
		DstNamespace: "dst_ns",
		DstNode:      "dst_node",
		DstZone:      "dst_zone",
		TxBytes:      10,
		TxPackets:    2,
		RxBytes:      30,
		RxPackets:    3,
		Proto:        "tcp",
		TS:           123,
	}

	jsBytes, err := jsoniter.Marshal(metric)
	r.NoError(err)

	jsBytesFast := marshalJSON(&metric)
	r.Equal(string(jsBytes)+"\n", string(jsBytesFast))
}

func BenchmarkMarshalJSONFast(b *testing.B) {
	metric := newTestMetric()

	b.Run("std json", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			js, err := json.Marshal(&metric)
			if err != nil {
				b.Fatal()
			}
			if len(js) == 0 {
				b.Fatal("0")
			}
		}
	})

	b.Run("jsoniter", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			js, err := jsoniter.Marshal(&metric)
			if err != nil {
				b.Fatal()
			}
			if len(js) == 0 {
				b.Fatal("0")
			}
		}
	})

	b.Run("fast json", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			js := marshalJSON(&metric)
			if len(js) == 0 {
				b.Fatal("0")
			}
		}
	})
}

type mockMetricsChanGetter struct {
	ch chan collector.PodNetworkMetric
}

func (m *mockMetricsChanGetter) GetMetricsChan() <-chan collector.PodNetworkMetric {
	return m.ch
}

func newTestMetric() collector.PodNetworkMetric {
	return collector.PodNetworkMetric{
		SrcIP:        "src_ip",
		SrcPod:       "src_pod",
		SrcNamespace: "src_ns",
		SrcNode:      "src_node",
		SrcZone:      "src_zone",
		DstIP:        "dst_ip",
		DstIPType:    "private",
		DstPod:       "dst_pod",
		DstNamespace: "dst_ns",
		DstNode:      "dst_node",
		DstZone:      "dst_zone",
		TxBytes:      10,
		TxPackets:    2,
		RxBytes:      30,
		RxPackets:    3,
		Proto:        "tcp",
		TS:           123,
	}
}
