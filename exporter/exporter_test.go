package exporter

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/castai/egressd/exporter/config"
	"github.com/castai/egressd/exporter/sinks"
	"github.com/castai/egressd/kube"
	"github.com/castai/egressd/pb"
)

func TestExporter(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	kubeWatcher := &mockKubeWatcher{
		pods: []*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "p1",
					Namespace: "team1",
				},
				Spec: corev1.PodSpec{
					NodeName: "n1",
				},
				Status: corev1.PodStatus{
					PodIP: "10.14.7.12",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "p2",
					Namespace: "team2",
				},
				Spec: corev1.PodSpec{
					NodeName: "n1",
				},
				Status: corev1.PodStatus{
					PodIP: "10.14.7.5",
				},
			},
		},
		nodes: []*corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "n1",
					Labels: map[string]string{
						"topology.kubernetes.io/zone": "us-east-1a",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
		},
	}
	collector1Srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		batch := &pb.RawNetworkMetricBatch{
			Items: []*pb.RawNetworkMetric{
				{
					SrcIp:     168691468,
					DstIp:     168691461,
					TxBytes:   35,
					TxPackets: 3,
					RxBytes:   30,
					RxPackets: 1,
					Proto:     6,
				},
			},
		}
		batchBytes, err := proto.Marshal(batch)
		r.NoError(err)
		_, err = w.Write(batchBytes)
		r.NoError(err)
	}))
	defer collector1Srv.Close()

	collectorURL, err := url.Parse(collector1Srv.URL)
	r.NoError(err)
	collectorHost, collectorPort, err := net.SplitHostPort(collectorURL.Host)
	r.NoError(err)
	collectorPortInt, err := strconv.Atoi(collectorPort)
	r.NoError(err)

	egressdCollectorPods := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app.kubernetes.io/component": "egressd-collector",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Ports: []corev1.ContainerPort{
							{
								Name:          "http-server",
								ContainerPort: int32(collectorPortInt), //nolint:gosec
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				PodIP: collectorHost,
			},
		},
	}
	kubeClient := fake.NewSimpleClientset(egressdCollectorPods[0])

	sink := &mockSink{batch: make(chan *pb.PodNetworkMetricBatch)}

	cfg := config.Config{
		ExportInterval: 100 * time.Millisecond,
		Sinks: map[string]config.Sink{
			"castai": {
				HTTPConfig: &config.SinkHTTPConfig{},
			},
		},
	}
	ex := New(log, cfg, kubeWatcher, kubeClient, []sinks.Sink{sink})

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		if err := ex.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error(err)
		}
	}()

	var pushedBatch *pb.PodNetworkMetricBatch
	select {
	case b := <-sink.batch:
		pushedBatch = b
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sink push")
	}

	r.Len(pushedBatch.Items, 1)
	item := pushedBatch.Items[0]
	r.Equal(&pb.PodNetworkMetric{
		SrcIp:        168691468,
		SrcPod:       "p1",
		SrcNamespace: "team1",
		SrcNode:      "n1",
		SrcZone:      "us-east-1a",
		DstIp:        168691461,
		DstPod:       "p2",
		DstNamespace: "team2",
		DstNode:      "n1",
		DstZone:      "us-east-1a",
		TxBytes:      35,
		TxPackets:    3,
		RxBytes:      30,
		RxPackets:    1,
		Proto:        6,
	}, item)
}

type mockKubeWatcher struct {
	pods  []*corev1.Pod
	nodes []*corev1.Node
}

func (m *mockKubeWatcher) GetNodeByIP(ip string) (*corev1.Node, error) {
	for _, node := range m.nodes {
		for _, addr := range node.Status.Addresses {
			if addr.Address == ip {
				return node, nil
			}
		}
	}
	return nil, kube.ErrNotFound
}

func (m *mockKubeWatcher) GetPodByIP(ip string) (*corev1.Pod, error) {
	for _, pod := range m.pods {
		if pod.Status.PodIP == ip {
			return pod, nil
		}
	}
	return nil, kube.ErrNotFound
}

func (m *mockKubeWatcher) GetNodeByName(name string) (*corev1.Node, error) {
	for _, node := range m.nodes {
		if node.Name == name {
			return node, nil
		}
	}
	return nil, kube.ErrNotFound
}

func (m *mockKubeWatcher) GetPodsByNode(nodeName string) ([]*corev1.Pod, error) {
	var res []*corev1.Pod
	for _, pod := range m.pods {
		if pod.Spec.NodeName == nodeName {
			res = append(res, pod)
		}
	}
	return res, nil
}

type mockSink struct {
	batch chan *pb.PodNetworkMetricBatch
}

func (m *mockSink) Push(ctx context.Context, batch *pb.PodNetworkMetricBatch) error {
	m.batch <- batch
	return nil
}
