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
			Src:       netaddr.MustParseIPPort("10.104.7.12:40001"),
			Dst:       netaddr.MustParseIPPort("10.104.7.5:3000"),
			TxBytes:   10,
			TxPackets: 1,
			RxBytes:   20,
			RxPackets: 2,
			Proto:     6,
			Ingress:   false,
		},
		{
			Src:       netaddr.MustParseIPPort("10.104.7.12:40002"),
			Dst:       netaddr.MustParseIPPort("10.104.7.5:3000"),
			TxBytes:   10,
			TxPackets: 1,
			RxBytes:   20,
			RxPackets: 2,
			Proto:     6,
			Ingress:   false,
		},
	}

	connTracker := &mockConntrack{
		entries: map[netaddr.IP][]conntrack.Entry{
			entries[0].Src.IP(): entries,
		},
	}
	kubeWatcher := &mockKubeWatcher{
		pods: []*corev1.Pod{
			{
				ObjectMeta: v1.ObjectMeta{
					Name: "p1",
				},
				Spec: corev1.PodSpec{
					NodeName: "n1",
				},
				Status: corev1.PodStatus{
					PodIP: "10.104.7.12",
				},
			},
		},
	}

	coll := New(Config{
		Interval:   time.Second,
		NodeName:   "n1",
		CacheItems: 1000,
	}, log, kubeWatcher, connTracker)

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

	r.Len(metrics, 1)
	// TODO: More asserts and more entries.
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
			Ingress:   false,
		},
		{
			Src:       netaddr.MustParseIPPort("10.104.7.12:40002"),
			Dst:       netaddr.MustParseIPPort("10.104.7.5:3000"),
			TxBytes:   10,
			TxPackets: 1,
			RxBytes:   20,
			RxPackets: 2,
			Proto:     6,
			Ingress:   false,
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
	entries map[netaddr.IP][]conntrack.Entry
}

func (m *mockConntrack) ListEntries(filter conntrack.EntriesFilter) (map[netaddr.IP][]conntrack.Entry, error) {
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
