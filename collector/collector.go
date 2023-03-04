package collector

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/maphash"
	"strings"
	"time"

	"github.com/castai/egressd/conntrack"
	"github.com/castai/egressd/kube"
	"github.com/castai/egressd/metrics"
	"github.com/sirupsen/logrus"
	"inet.af/netaddr"
	corev1 "k8s.io/api/core/v1"
)

func CurrentTimeGetter() func() time.Time {
	return func() time.Time {
		return time.Now()
	}
}

type Config struct {
	// ReadInterval used for conntrack records scrape.
	ReadInterval time.Duration
	// FlushInterval used for aggregated metrics export.
	FlushInterval time.Duration
	// CleanupInterval used to remove expired conntrack and pod metrics records.
	CleanupInterval time.Duration
	// NodeName is current node name on which egressd is running.
	NodeName string
	// ExcludeNamespaces allows to exclude namespaces. Input is comma separated string.
	ExcludeNamespaces string
	// MetricBufferSize export channel buffer size.
	MetricBufferSize int
}

func New(
	cfg Config,
	log logrus.FieldLogger,
	kubeWatcher kube.Watcher,
	conntracker conntrack.Client,
	currentTimeGetter func() time.Time,
	dnsStore DNSStore,
) *Collector {
	excludeNsMap := map[string]struct{}{}
	if cfg.ExcludeNamespaces != "" {
		nsList := strings.Split(cfg.ExcludeNamespaces, ",")
		for _, ns := range nsList {
			excludeNsMap[ns] = struct{}{}
		}
	}
	if cfg.ReadInterval == 0 {
		panic("read interval not set")
	}
	if cfg.FlushInterval == 0 {
		panic("flush interval not set")
	}
	if cfg.CleanupInterval == 0 {
		panic("cleanup interval not set")
	}

	return &Collector{
		cfg:               cfg,
		log:               log,
		kubeWatcher:       kubeWatcher,
		conntracker:       conntracker,
		entriesCache:      make(map[uint64]*conntrack.Entry),
		podMetrics:        map[uint64]*PodNetworkMetric{},
		metricsChan:       make(chan *PodNetworkMetric, cfg.MetricBufferSize),
		excludeNsMap:      excludeNsMap,
		currentTimeGetter: currentTimeGetter,
		dnsStore:          dnsStore,
	}
}

type Collector struct {
	cfg               Config
	log               logrus.FieldLogger
	kubeWatcher       kube.Watcher
	conntracker       conntrack.Client
	entriesCache      map[uint64]*conntrack.Entry
	podMetrics        map[uint64]*PodNetworkMetric
	excludeNsMap      map[string]struct{}
	metricsChan       chan *PodNetworkMetric
	currentTimeGetter func() time.Time
	dnsStore          DNSStore
}

func (c *Collector) GetMetricsChan() <-chan *PodNetworkMetric {
	return c.metricsChan
}

func (c *Collector) Start(ctx context.Context) error {
	readTicker := time.NewTicker(c.cfg.ReadInterval)
	exportTicker := time.NewTicker(c.cfg.FlushInterval)
	cleanupTicker := time.NewTicker(c.cfg.CleanupInterval)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-readTicker.C:
			if err := c.collect(); err != nil {
				c.log.Errorf("collecting: %v", err)
			}
		case <-exportTicker.C:
			c.export()
		case <-cleanupTicker.C:
			c.cleanup()
		}
	}
}

// collect aggregates conntract records into reduced pod metrics.
func (c *Collector) collect() error {
	start := time.Now()
	pods, err := c.getNodePods()
	if err != nil {
		return fmt.Errorf("getting node pods: %w", err)
	}
	conns, err := c.conntracker.ListEntries(conntrack.FilterBySrcIP(getPodIPs(pods)))
	if err != nil {
		return fmt.Errorf("listing conntrack entries: %w", err)
	}
	metrics.SetConntrackActiveEntriesCount(float64(len(conns)))

	for _, conn := range conns {
		connKey := conntrackEntryKey(conn)
		txBytes := conn.TxBytes
		txPackets := conn.TxPackets
		rxBytes := conn.RxBytes
		rxPackets := conn.RxPackets

		if cachedConn, found := c.entriesCache[connKey]; found {
			txBytes -= cachedConn.TxBytes
			txPackets -= cachedConn.TxPackets
			rxBytes -= cachedConn.RxBytes
			rxPackets -= cachedConn.RxPackets
		}
		c.entriesCache[connKey] = conn

		groupKey := entryGroupKey(conn)
		if pm, found := c.podMetrics[groupKey]; found {
			pm.TxBytes += txBytes
			pm.TxPackets += txPackets
			pm.RxBytes += rxBytes
			pm.RxPackets += rxPackets
			if conn.LifetimeUnixSeconds > pm.lifetimeUnixSeconds {
				pm.lifetimeUnixSeconds = conn.LifetimeUnixSeconds
			}
		} else {
			pm, err := c.initNewPodNetworkMetric(conn)
			if err != nil {
				c.log.Warnf("initializing new pod network metric: %v", err)
				continue
			}
			c.podMetrics[groupKey] = pm
		}
	}

	c.log.Debugf("collection done in %s, pods=%d, conntrack=%d, conntrack_cache=%d", time.Since(start), len(pods), len(conns), len(c.entriesCache))
	return nil
}

func (c *Collector) initNewPodNetworkMetric(conn *conntrack.Entry) (*PodNetworkMetric, error) {
	srcIPString := conn.Src.IP().String()
	pod, err := c.kubeWatcher.GetPodByIP(srcIPString)
	if err != nil {
		return nil, fmt.Errorf("getting pod by ip %q: %w", srcIPString, err)
	}
	dstIPString := conn.Dst.IP().String()
	metric := PodNetworkMetric{
		SrcIP:               conn.Src.IP().String(),
		SrcPod:              pod.Name,
		SrcNamespace:        pod.Namespace,
		SrcNode:             pod.Spec.NodeName,
		DstIP:               dstIPString,
		DstIPType:           ipType(conn.Dst.IP()),
		TxBytes:             conn.TxBytes,
		TxPackets:           conn.TxPackets,
		RxBytes:             conn.RxBytes,
		RxPackets:           conn.RxPackets,
		Proto:               conntrack.ProtoString(conn.Proto),
		lifetimeUnixSeconds: conn.LifetimeUnixSeconds,
	}

	srcNode, err := c.kubeWatcher.GetNodeByName(pod.Spec.NodeName)
	if err != nil && !errors.Is(err, kube.ErrNotFound) {
		return nil, err
	}
	if srcNode != nil {
		metric.SrcZone = getNodeZone(srcNode)
	}

	// Try to find destination pod and node info.
	if conn.Dst.IP().IsPrivate() {
		dstIP := dstIPString
		// First try finding destination pod by ip.
		dstPod, err := c.kubeWatcher.GetPodByIP(dstIP)
		if err != nil && !errors.Is(err, kube.ErrNotFound) && !errors.Is(err, kube.ErrToManyObjects) {
			return nil, err
		}
		if dstPod != nil {
			metric.DstPod = dstPod.Name
			metric.DstNamespace = dstPod.Namespace

			// Also find destination node by name.
			dstNode, err := c.kubeWatcher.GetNodeByName(dstPod.Spec.NodeName)
			if err != nil && !errors.Is(err, kube.ErrNotFound) {
				return nil, err
			}
			if dstNode != nil {
				metric.DstNode = dstNode.Name
				metric.DstZone = getNodeZone(dstNode)
			}
		} else {
			// No destination pod found. But at least we can try finding destination node.
			dstNode, err := c.kubeWatcher.GetNodeByIP(dstIP)
			if err != nil && !errors.Is(err, kube.ErrNotFound) {
				return nil, err
			}
			if dstNode != nil {
				metric.DstNode = dstNode.Name
				metric.DstZone = getNodeZone(dstNode)
			}
		}
	}
	return &metric, nil
}

func (c *Collector) export() {
	start := time.Now()
	ts := uint64(c.currentTimeGetter().UTC().UnixMilli())

	metricsCount := 0
	for _, metric := range c.podMetrics {
		metric.TS = ts
		select {
		case c.metricsChan <- metric:
			metricsCount++
		default:
			metrics.IncDroppedEvents()
			c.log.Errorf("dropping metric event, channel is full. "+
				"Consider increasing --metrics-buffer-size from current value: %d", c.cfg.MetricBufferSize)
		}
	}

	c.log.Infof("flushed in %s, metrics=%d", time.Since(start), metricsCount)
}

func (c *Collector) cleanup() {
	start := time.Now()
	nowUnixSeconds := getUnixNowSeconds(c.currentTimeGetter)
	deletedEntriesCount := 0
	deletedPodMetricsCount := 0

	for key, e := range c.entriesCache {
		if e.LifetimeUnixSeconds < nowUnixSeconds {
			delete(c.entriesCache, key)
			deletedEntriesCount++
		}
	}

	for key, m := range c.podMetrics {
		if m.lifetimeUnixSeconds < nowUnixSeconds {
			delete(c.podMetrics, key)
			deletedPodMetricsCount++
		}
	}

	c.log.Infof("cleanup done in %s, deleted_conntrack=%d, deleted_pod_metrics=%d", time.Since(start), deletedEntriesCount, deletedPodMetricsCount)
}

func (c *Collector) getNodePods() ([]*corev1.Pod, error) {
	pods, err := c.kubeWatcher.GetPodsByNode(c.cfg.NodeName)
	if err != nil {
		return nil, err
	}
	filtered := pods[:0]
	for _, pod := range pods {
		podIP := pod.Status.PodIP
		if podIP == "" {
			continue
		}
		// Don't track host network pods since we don't have enough info in conntrack.
		if pod.Spec.HostNetwork {
			continue
		}
		if _, found := c.excludeNsMap[pod.Namespace]; found {
			continue
		}
		filtered = append(filtered, pod)
	}
	return filtered, nil
}

func getPodIPs(pods []*corev1.Pod) map[netaddr.IP]struct{} {
	ips := make(map[netaddr.IP]struct{}, len(pods))
	for _, pod := range pods {
		ips[netaddr.MustParseIP(pod.Status.PodIP)] = struct{}{}
	}
	return ips
}

var entryGroupHash maphash.Hash

// entryGroupKey groups by src, dst and port.
func entryGroupKey(conn *conntrack.Entry) uint64 {
	srcIP := conn.Src.IP().As4()
	_, _ = entryGroupHash.Write(srcIP[:])
	dstIP := conn.Dst.IP().As4()
	_, _ = entryGroupHash.Write(dstIP[:])
	_ = entryGroupHash.WriteByte(conn.Proto)
	res := entryGroupHash.Sum64()
	entryGroupHash.Reset()
	return res
}

var conntrackEntryHash maphash.Hash

func conntrackEntryKey(conn *conntrack.Entry) uint64 {
	srcIP := conn.Src.IP().As4()
	_, _ = conntrackEntryHash.Write(srcIP[:])
	var srcPort [2]byte
	binary.LittleEndian.PutUint16(srcPort[:], conn.Src.Port())
	_, _ = conntrackEntryHash.Write(srcPort[:])

	dstIP := conn.Dst.IP().As4()
	_, _ = conntrackEntryHash.Write(dstIP[:])
	var dstPort [2]byte
	binary.LittleEndian.PutUint16(dstPort[:], conn.Dst.Port())
	_, _ = conntrackEntryHash.Write(dstPort[:])

	_ = conntrackEntryHash.WriteByte(conn.Proto)
	res := conntrackEntryHash.Sum64()

	conntrackEntryHash.Reset()
	return res
}

func getNodeZone(node *corev1.Node) string {
	return node.Labels["topology.kubernetes.io/zone"]
}

func ipType(ip netaddr.IP) string {
	if ip.IsPrivate() {
		return "private"
	}
	return "public"
}

func getUnixNowSeconds(now func() time.Time) uint32 {
	return uint32(now().Unix())
}
