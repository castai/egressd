package main

import (
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/castai/egressd/pb"
)

var (
	imageTag = flag.String("image-tag", "", "Egressd docker image tag")
	timeout  = flag.Duration("timeout", 90*time.Second, "Test timeout")
	ns       = flag.String("ns", "castai-egressd-e2e", "Namespace")
)

func main() {
	flag.Parse()
	log := logrus.New()
	if err := run(log); err != nil {
		log.Error(err)
		time.Sleep(5 * time.Second)
		os.Exit(-1)
	}
}

func run(log logrus.FieldLogger) error {
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if *imageTag == "" {
		return errors.New("image-tag flag is not set")
	}

	api := &mockAPI{log: log}
	go api.start()

	out, err := installChart(*ns, *imageTag)
	if err != nil {
		return fmt.Errorf("installing chart: %w: %s", err, string(out))
	}
	fmt.Printf("installed chart:\n%s\n", out)

	if err := api.assertMetricsReceived(ctx); err != nil {
		return err
	}

	return nil
}

func installChart(ns, imageTag string) ([]byte, error) {
	fmt.Printf("installing egressd chart with image tag %q", imageTag)
	podIP := os.Getenv("POD_IP")
	apiURL := fmt.Sprintf("http://%s:8090", podIP)
	repo := "ghcr.io/castai/egressd/egressd"
	if imageTag == "local" {
		repo = "egressd"
	}
	//nolint:gosec
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf(
		`helm upgrade --install castai-egressd ./charts/egressd \
  -n %s --create-namespace \
  --set collector.image.repository=%s \
  --set collector.image.tag=%s \
  --set collector.extraArgs.log-level=debug \
  --set collector.enableDnsTrace=false \
  --set exporter.image.repository=%s \
  --set exporter.image.tag=%s \
  --set exporter.structuredConfig.exportInterval=10s \
  --set exporter.extraArgs.log-level=debug \
  --set castai.apiURL=%s \
  --set castai.clusterID=e2e \
  --set castai.apiKey=key \
  --wait --timeout=1m`,
		ns,
		repo,
		imageTag,
		repo+"-exporter",
		imageTag,
		apiURL,
	))
	return cmd.CombinedOutput()
}

type mockAPI struct {
	log logrus.FieldLogger

	mu                          sync.Mutex
	receivedMetricsBatchesBytes [][]byte
}

func (m *mockAPI) start() {
	router := mux.NewRouter()

	router.HandleFunc("/v1/security/egress/{cluster_id}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		clusterID := vars["cluster_id"]
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			m.log.Errorf("gzip reader: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer gz.Close()

		rawBody, err := io.ReadAll(gz)
		if err != nil {
			m.log.Errorf("read body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		fmt.Printf("received metrics batch, cluster_id=%s, size_bytes=%d\n", clusterID, len(rawBody))
		m.mu.Lock()
		m.receivedMetricsBatchesBytes = append(m.receivedMetricsBatchesBytes, rawBody)
		m.mu.Unlock()

		w.WriteHeader(http.StatusAccepted)
	})

	if err := http.ListenAndServe(":8090", router); err != nil { //nolint:gosec
		m.log.Fatal(err)
	}
}

func (m *mockAPI) getPodNetworkMetrics() ([]*pb.PodNetworkMetric, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var res []*pb.PodNetworkMetric
	for _, chunk := range m.receivedMetricsBatchesBytes {
		var batch pb.PodNetworkMetricBatch
		if err := proto.Unmarshal(chunk, &batch); err != nil {
			return nil, err
		}
		res = append(res, batch.Items...)
	}
	return res, nil
}

func (m *mockAPI) assertMetricsReceived(ctx context.Context) error {
	for {
		select {
		case <-time.After(3 * time.Second):
			logs, err := m.getPodNetworkMetrics()
			if err != nil {
				return err
			}
			if len(logs) == 0 {
				continue
			}
			logEntry := logs[0]
			if logEntry.SrcIp == "" {
				return errors.New("source ip is missing")
			}
			if logEntry.DstIp == "" {
				return errors.New("dest ip is missing")
			}
			return nil
		case <-ctx.Done():
			return fmt.Errorf("waiting for received logs: %w", ctx.Err())
		}
	}
}
