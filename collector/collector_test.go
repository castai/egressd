package collector

import (
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/castai/egressd/conntrack"
	"github.com/castai/egressd/kube"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"inet.af/netaddr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCollector(t *testing.T) {
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
		},
	}

	newCollector := func(connTracker conntrack.Client) *Collector {
		return New(Config{
			ReadInterval:     time.Millisecond,
			FlushInterval:    2 * time.Millisecond,
			CleanupInterval:  3 * time.Millisecond,
			NodeName:         "n1",
			MetricBufferSize: 1000,
		}, log,
			kubeWatcher,
			connTracker,
			mockTimeGetter,
		)
	}

	t.Run("basic flow", func(t *testing.T) {
		r := require.New(t)

		// Initially conntrack entries.
		initialEntries := []conntrack.Entry{
			{
				Src:       netaddr.MustParseIPPort("10.14.7.12:40001"),
				Dst:       netaddr.MustParseIPPort("10.14.7.5:3000"),
				TxBytes:   10,
				TxPackets: 1,
				Proto:     6,
			},
			{
				Src:       netaddr.MustParseIPPort("10.14.7.12:40002"),
				Dst:       netaddr.MustParseIPPort("10.14.7.5:3000"),
				TxBytes:   15,
				TxPackets: 1,
				Proto:     6,
			},
		}

		connTracker := &mockConntrack{
			entries: initialEntries,
		}

		coll := newCollector(connTracker)

		var metrics []PodNetworkMetric
		done := make(chan struct{})
		go func() {
			for e := range coll.GetMetricsChan() {
				metrics = append(metrics, *e)
			}
			done <- struct{}{}
		}()

		// Collect first time.
		r.NoError(coll.collect())

		// Update tx stats.
		initialEntries[0].TxBytes += 10
		initialEntries[0].TxPackets += 1
		r.NoError(coll.collect())

		// Add rx stats.
		initialEntries[0].RxBytes += 30
		initialEntries[0].RxPackets += 1
		r.NoError(coll.collect())

		// Add new entry for another protocol.
		connTracker.entries = append(connTracker.entries, conntrack.Entry{
			Src:       netaddr.MustParseIPPort("10.14.7.12:40002"),
			Dst:       netaddr.MustParseIPPort("10.14.7.5:53"),
			TxBytes:   40,
			TxPackets: 1,
			Proto:     17,
		})
		r.NoError(coll.collect())

		// Simulate entry expire.
		connTracker.entries = initialEntries
		r.NoError(coll.collect())

		// Flush and export metrics.
		coll.export()

		close(coll.metricsChan)
		<-done

		sort.Slice(metrics, func(i, j int) bool {
			return metrics[i].Proto < metrics[j].Proto
		})

		r.Len(metrics, 2)
		m1 := metrics[0]
		r.Equal(PodNetworkMetric{
			SrcIP:        "10.14.7.12",
			SrcPod:       "p1",
			SrcNamespace: "team1",
			SrcNode:      "n1",
			DstIP:        "10.14.7.5",
			DstIPType:    "private",
			TxBytes:      35,
			TxPackets:    3,
			RxBytes:      30,
			RxPackets:    1,
			Proto:        "TCP",
			TS:           1577840461000,
		}, m1)

		m2 := metrics[1]
		r.Equal(PodNetworkMetric{
			SrcIP:        "10.14.7.12",
			SrcPod:       "p1",
			SrcNamespace: "team1",
			SrcNode:      "n1",
			DstIP:        "10.14.7.5",
			DstIPType:    "private",
			TxBytes:      40,
			TxPackets:    1,
			RxBytes:      0,
			RxPackets:    0,
			Proto:        "UDP",
			TS:           1577840461000,
		}, m2)
	})

	t.Run("multiple collect with no new entries", func(t *testing.T) {
		r := require.New(t)

		connTracker := &mockConntrack{
			entries: []conntrack.Entry{
				{
					Src:       netaddr.MustParseIPPort("10.14.7.12:40001"),
					Dst:       netaddr.MustParseIPPort("10.14.7.5:3000"),
					TxBytes:   10,
					TxPackets: 1,
					Proto:     6,
				},
			},
		}

		coll := newCollector(connTracker)

		r.NoError(coll.collect())
		r.NoError(coll.collect())
		r.Len(coll.podMetrics, 1)
		metric := lo.Values(coll.podMetrics)[0]
		r.Equal(10, int(metric.TxBytes))
		r.Equal(1, int(metric.TxPackets))
	})

	t.Run("update metric with latest conntrack lifetimeUnixSeconds", func(t *testing.T) {
		r := require.New(t)

		connTracker := &mockConntrack{
			entries: []conntrack.Entry{
				{
					Src:                 netaddr.MustParseIPPort("10.14.7.12:40001"),
					Dst:                 netaddr.MustParseIPPort("10.14.7.5:3000"),
					TxBytes:             10,
					TxPackets:           1,
					Proto:               6,
					LifetimeUnixSeconds: 5, // This expiration should be used.
				},
				{
					Src:                 netaddr.MustParseIPPort("10.14.7.12:40001"),
					Dst:                 netaddr.MustParseIPPort("10.14.7.5:3000"),
					TxBytes:             10,
					TxPackets:           1,
					Proto:               6,
					LifetimeUnixSeconds: 1,
				},
			},
		}

		coll := newCollector(connTracker)

		r.NoError(coll.collect())
		r.NoError(coll.collect())
		r.Len(coll.podMetrics, 1)
		metric := lo.Values(coll.podMetrics)[0]
		r.Equal(5, int(metric.lifetimeUnixSeconds))
	})

	t.Run("cleanup expired", func(t *testing.T) {
		r := require.New(t)

		nowUnixSeconds := uint32(mockTimeGetter().Unix())

		connTracker := &mockConntrack{}
		coll := newCollector(connTracker)
		coll.entriesCache[0] = &conntrack.Entry{LifetimeUnixSeconds: nowUnixSeconds + 10000}
		coll.entriesCache[1] = &conntrack.Entry{LifetimeUnixSeconds: nowUnixSeconds - 100}
		coll.podMetrics[0] = &PodNetworkMetric{lifetimeUnixSeconds: nowUnixSeconds + 10000}
		coll.podMetrics[1] = &PodNetworkMetric{lifetimeUnixSeconds: nowUnixSeconds - 150}

		coll.cleanup()
		entries := lo.Values(coll.entriesCache)
		metrics := lo.Values(coll.podMetrics)
		r.Len(entries, 1)
		r.Len(metrics, 1)
	})
}

func BenchmarkCollector(b *testing.B) {
	log := logrus.New()
	podsCount := 100
	connsPerPod := 1000
	var pods []*corev1.Pod
	var conns []conntrack.Entry
	for i := 0; i < podsCount; i++ {
		podIP := "10.14.7." + strconv.Itoa(i)
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod" + strconv.Itoa(i),
				Namespace: "ns",
			},
			Spec: corev1.PodSpec{
				NodeName: "n1",
			},
			Status: corev1.PodStatus{
				PodIP: podIP,
			},
		})
		c, d := uint8(0), uint8(0)
		for i := 0; i < connsPerPod; i++ {
			d++
			if d == 255 {
				d = 0
				c++
			}
			conns = append(conns, conntrack.Entry{
				Src:                 netaddr.IPPortFrom(netaddr.MustParseIP(podIP), uint16(i)),
				Dst:                 netaddr.IPPortFrom(netaddr.IPv4(10, 10, c, d), uint16(i)),
				TxBytes:             uint64(i),
				TxPackets:           uint64(i),
				RxBytes:             uint64(i),
				RxPackets:           uint64(i),
				LifetimeUnixSeconds: 0,
				Proto:               6,
			})
		}
	}
	kubeWatcher := &mockKubeWatcher{
		pods: pods,
	}
	connTracker := &mockConntrack{entries: conns}
	coll := New(Config{
		ReadInterval:     time.Millisecond,
		FlushInterval:    2 * time.Millisecond,
		CleanupInterval:  3 * time.Millisecond,
		NodeName:         "n1",
		MetricBufferSize: 1000,
	}, log,
		kubeWatcher,
		connTracker,
		mockTimeGetter,
	)
	expectedConns := podsCount * connsPerPod
	expectedMetrics := podsCount * connsPerPod

	b.Run("collect", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := coll.collect(); err != nil {
				b.Fatal(err)
			}
			if l := len(coll.entriesCache); l != expectedConns {
				b.Fatalf("expected conns %d, got %d", expectedConns, l)
			}
			if l := len(coll.podMetrics); l != expectedMetrics {
				b.Fatalf("expected conns %d, got %d", expectedMetrics, l)
			}
		}
	})
}

func mockTimeGetter() time.Time {
	return time.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
}

type mockConntrack struct {
	entries []conntrack.Entry
}

func (m *mockConntrack) ListEntries(filter conntrack.EntriesFilter) ([]*conntrack.Entry, error) {
	out := make([]*conntrack.Entry, 0, len(m.entries))
	for _, e := range m.entries {
		e := e
		if filter(&e) {
			out = append(out, &e)
		}
	}
	return out, nil
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
