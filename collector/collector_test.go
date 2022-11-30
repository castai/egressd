package collector

import (
	"context"
	"testing"

	"github.com/castai/egressd/conntrack"
	"github.com/castai/egressd/kube"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"inet.af/netaddr"
	corev1 "k8s.io/api/core/v1"
)

func TestCollector(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	connTracker := &mockConntrack{}
	kubeWatcher := &mockKubeWatcher{}

	coll := New(Config{
		Interval: 0,
		NodeName: "",
	}, log, kubeWatcher, connTracker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runerr := make(chan error)
	go func() {
		runerr <- coll.Start(ctx)
	}()
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
			Reply:     false,
		},
		{
			Src:       netaddr.MustParseIPPort("10.104.7.12:40002"),
			Dst:       netaddr.MustParseIPPort("10.104.7.5:3000"),
			TxBytes:   10,
			TxPackets: 1,
			RxBytes:   20,
			RxPackets: 2,
			Proto:     6,
			Reply:     false,
		},
	}
	grouped, err := groupConns(entries)
	r.NoError(err)
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

func (m *mockConntrack) ListEntries() (map[netaddr.IP][]conntrack.Entry, error) {
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
	//TODO implement me
	panic("implement me")
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
