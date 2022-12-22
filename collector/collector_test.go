package collector

import (
	"testing"
	"time"

	"github.com/castai/egressd/conntrack"
	"github.com/castai/egressd/kube"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"inet.af/netaddr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCollector(t *testing.T) {
	r := require.New(t)
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	entries := []conntrack.Entry{
		{
			Src:       netaddr.MustParseIPPort("10.14.7.12:40001"),
			Dst:       netaddr.MustParseIPPort("10.14.7.5:3000"),
			TxBytes:   10,
			TxPackets: 1,
			RxBytes:   20,
			RxPackets: 2,
			Proto:     6,
		},
		{
			Src:       netaddr.MustParseIPPort("10.14.7.12:40002"),
			Dst:       netaddr.MustParseIPPort("10.14.7.5:3000"),
			TxBytes:   10,
			TxPackets: 1,
			RxBytes:   20,
			RxPackets: 2,
			Proto:     6,
		},
		{
			Src:       netaddr.MustParseIPPort("1.1.1.1:40002"),
			Dst:       netaddr.MustParseIPPort("10.20.3.34:80"),
			TxBytes:   101,
			TxPackets: 2,
			RxBytes:   201,
			RxPackets: 3,
			Proto:     6,
		},
	}

	connTracker := &mockConntrack{
		entries: entries,
	}
	kubeWatcher := &mockKubeWatcher{
		pods: []*corev1.Pod{
			{
				ObjectMeta: v1.ObjectMeta{
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
				ObjectMeta: v1.ObjectMeta{
					Name:      "p2",
					Namespace: "team2",
				},
				Spec: corev1.PodSpec{
					NodeName: "n2",
				},
				Status: corev1.PodStatus{
					PodIP: "10.14.7.5",
				},
			},
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "p3",
					Namespace: "team3",
				},
				Spec: corev1.PodSpec{
					NodeName: "n1",
				},
				Status: corev1.PodStatus{
					PodIP: "10.20.3.34",
				},
			},
		},
		nodes: []*corev1.Node{
			{
				ObjectMeta: v1.ObjectMeta{
					Name: "n1",
					Labels: map[string]string{
						"topology.kubernetes.io/zone": "us-east1-a",
					},
				},
			},
			{
				ObjectMeta: v1.ObjectMeta{
					Name: "n2",
					Labels: map[string]string{
						"topology.kubernetes.io/zone": "us-east1-b",
					},
				},
			},
		},
	}

	coll := New(Config{
		Interval:   time.Second,
		NodeName:   "n1",
		CacheItems: 1000,
	}, log,
		kubeWatcher,
		connTracker,
		func() time.Time {
			return time.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
		})

	var metrics []PodNetworkMetric
	done := make(chan struct{})
	go func() {
		for e := range coll.GetMetricsChan() {
			metrics = append(metrics, e)
		}
		done <- struct{}{}
	}()

	err := coll.run()
	r.NoError(err)
	err = coll.run()
	r.NoError(err)
	close(coll.metricsChan)
	<-done

	// First entry is summed, second is not added as it didn't change
	r.Len(metrics, 2)
	m1 := metrics[0]
	r.Equal(PodNetworkMetric{
		SrcIP:        "10.14.7.12",
		SrcPod:       "p1",
		SrcNamespace: "team1",
		SrcNode:      "n1",
		SrcZone:      "us-east1-a",
		DstIP:        "10.14.7.5",
		DstIPType:    "private",
		DstPod:       "p2",
		DstNamespace: "team2",
		DstNode:      "n2",
		DstZone:      "us-east1-b",
		TxBytes:      20,
		TxPackets:    2,
		RxBytes:      40,
		RxPackets:    4,
		Proto:        "TCP",
		TS:           1577840461000,
	}, m1)

	m2 := metrics[1]
	r.Equal(PodNetworkMetric{
		SrcIP:        "10.20.3.34",
		SrcPod:       "p3",
		SrcNamespace: "team3",
		SrcNode:      "n1",
		SrcZone:      "us-east1-a",
		DstIP:        "1.1.1.1",
		DstIPType:    "public",
		DstPod:       "",
		DstNamespace: "",
		DstNode:      "",
		DstZone:      "",
		TxBytes:      201,
		TxPackets:    3,
		RxBytes:      101,
		RxPackets:    3,
		Proto:        "TCP",
		TS:           1577840461000,
	}, m2)
}

func TestGroupPodConns(t *testing.T) {
	r := require.New(t)

	entries := []conntrack.Entry{
		{
			Src:       netaddr.MustParseIPPort("10.104.7.12:40001"),
			Dst:       netaddr.MustParseIPPort("10.104.7.5:3000"),
			TxBytes:   10,
			TxPackets: 1,
			RxBytes:   20,
			RxPackets: 2,
			Proto:     6,
		},
		{
			Src:       netaddr.MustParseIPPort("10.104.7.12:40002"),
			Dst:       netaddr.MustParseIPPort("10.104.7.5:3000"),
			TxBytes:   10,
			TxPackets: 1,
			RxBytes:   20,
			RxPackets: 2,
			Proto:     6,
		},
	}
	grouped := groupConns(entries)
	r.Len(grouped, 1)
	var item *groupedConn
	for _, v := range grouped {
		item = v
	}
	r.Equal(uint64(20), item.txBytes)
	r.Equal(uint64(2), item.txPackets)
}

type mockConntrack struct {
	entries []conntrack.Entry
}

func (m *mockConntrack) ListEntries(filter conntrack.EntriesFilter) ([]conntrack.Entry, error) {
	return m.entries, nil
}

func (m *mockConntrack) Close() error {
	return nil
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
