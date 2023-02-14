package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/castai/egressd/collector"
	jsoniter "github.com/json-iterator/go"
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

func BenchmarkEncoding(b *testing.B) {
	n := 10000
	items := make([]*collector.PodNetworkMetric, n)
	for i := 0; i < n; i++ {
		s := strconv.Itoa(i)
		items[i] = &collector.PodNetworkMetric{
			SrcIP:        "10.10.0." + s,
			SrcPod:       "source-pod-nginx-demo-app-" + s,
			SrcNamespace: "source-namespace",
			SrcNode:      "source-node-123" + s,
			SrcZone:      "us-east-5",
			DstIP:        "20.10.0." + s,
			DstIPType:    "private",
			DstPod:       "dest-pod-demo-app-123-" + s,
			DstNamespace: "dst-pod-namespace",
			DstNode:      "dst-node-name-1234afew-" + s,
			DstZone:      "us-centra-999",
			TxBytes:      10000 + uint64(i),
			TxPackets:    10000 + uint64(i),
			RxBytes:      10000 + uint64(i),
			RxPackets:    10000 + uint64(i),
			Proto:        "TCP",
			TS:           123232432432432,
		}
	}

	itemsJSONLarge, err := json.Marshal(items)
	if err != nil {
		b.Fatal(err)
	}

	itemsJSONMedium, err := json.Marshal(items[:5000])
	if err != nil {
		b.Fatal(err)
	}

	itemsJSONSmall, err := json.Marshal(items[:1000])
	if err != nil {
		b.Fatal(err)
	}

	b.Run("marshal", func(b *testing.B) {
		b.Run("marshal single event with std json", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				bb, err := json.Marshal(items[0])
				if err != nil {
					b.Fatal(err)
				}
				if len(bb) == 0 {
					b.Fatal("no bytes")
				}
			}
		})

		b.Run("marshal single event using encoder with std json", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var buf bytes.Buffer
				err := json.NewEncoder(&buf).Encode(items[0])
				if err != nil {
					b.Fatal(err)
				}
				if buf.Len() == 0 {
					b.Fatal("no bytes")
				}
			}
		})

		b.Run("marshal single event with jsoniter", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				bb, err := jsoniter.Marshal(items[0])
				if err != nil {
					b.Fatal(err)
				}
				if len(bb) == 0 {
					b.Fatal("no bytes")
				}
			}
		})

		b.Run("marshal single event with fast jsoniter", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				bb, err := jsoniter.ConfigFastest.Marshal(items[0])
				if err != nil {
					b.Fatal(err)
				}
				if len(bb) == 0 {
					b.Fatal("no bytes")
				}
			}
		})

		b.Run("marshal single event using encoder with jsoniter", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var buf bytes.Buffer
				err := jsoniter.NewEncoder(&buf).Encode(items[0])
				if err != nil {
					b.Fatal(err)
				}
				if buf.Len() == 0 {
					b.Fatal("no bytes")
				}
			}
		})
	})

	b.Run("unmarshal", func(b *testing.B) {
		tests := []struct {
			name  string
			items []byte
		}{
			{
				name:  "large",
				items: itemsJSONLarge,
			},
			{
				name:  "medium",
				items: itemsJSONMedium,
			},
			{
				name:  "small",
				items: itemsJSONSmall,
			},
		}

		for _, test := range tests {
			test := test
			b.Run(fmt.Sprintf("unmarshal %s json with std", test.name), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					var res []*collector.PodNetworkMetric
					if err := json.Unmarshal(test.items, &res); err != nil {
						b.Fatal(err)
					}
					if len(res) == 0 {
						b.Fatal("no items")
					}
				}
			})

			b.Run(fmt.Sprintf("unmarshal %s json with jsoniter", test.name), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					var res []*collector.PodNetworkMetric
					if err := jsoniter.Unmarshal(test.items, &res); err != nil {
						b.Fatal(err)
					}
					if len(res) == 0 {
						b.Fatal("no items")
					}
				}
			})

			b.Run(fmt.Sprintf("unmarshal %s json with fast jsoniter", test.name), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					var res []*collector.PodNetworkMetric
					if err := jsoniter.ConfigFastest.Unmarshal(test.items, &res); err != nil {
						b.Fatal(err)
					}
					if len(res) == 0 {
						b.Fatal("no items")
					}
				}
			})
		}
	})
}
