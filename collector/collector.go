package collector

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/maphash"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"inet.af/netaddr"
	corev1 "k8s.io/api/core/v1"

	"github.com/castai/egressd/conntrack"
	"github.com/castai/egressd/metrics"
	"github.com/castai/egressd/pb"
)

func CurrentTimeGetter() func() time.Time {
	return func() time.Time {
		return time.Now()
	}
}

type Config struct {
	// ReadInterval used for conntrack records scrape.
	ReadInterval time.Duration
	// CleanupInterval used to remove expired conntrack and pod metrics records.
	CleanupInterval time.Duration
	// NodeName is current node name on which egressd is running.
	NodeName string
	// ExcludeNamespaces allows to exclude namespaces. Input is comma separated string.
	ExcludeNamespaces string
}

type podsWatcher interface {
	Get(nodeName string) ([]*corev1.Pod, error)
}

type rawNetworkMetric struct {
	*pb.RawNetworkMetric
	lifetime time.Time
}

func New(
	cfg Config,
	log logrus.FieldLogger,
	podsWatcher podsWatcher,
	conntracker conntrack.Client,
	currentTimeGetter func() time.Time,
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
	if cfg.CleanupInterval == 0 {
		panic("cleanup interval not set")
	}

	return &Collector{
		cfg:               cfg,
		log:               log,
		podsWatcher:       podsWatcher,
		conntracker:       conntracker,
		entriesCache:      make(map[uint64]*conntrack.Entry),
		podMetrics:        map[uint64]*rawNetworkMetric{},
		excludeNsMap:      excludeNsMap,
		currentTimeGetter: currentTimeGetter,
		exporterClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

type Collector struct {
	cfg               Config
	log               logrus.FieldLogger
	podsWatcher       podsWatcher
	conntracker       conntrack.Client
	entriesCache      map[uint64]*conntrack.Entry
	podMetrics        map[uint64]*rawNetworkMetric
	excludeNsMap      map[string]struct{}
	currentTimeGetter func() time.Time
	exporterClient    *http.Client
	mu                sync.Mutex
}

func (c *Collector) Start(ctx context.Context) error {
	readTicker := time.NewTicker(c.cfg.ReadInterval)
	cleanupTicker := time.NewTicker(c.cfg.CleanupInterval)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-readTicker.C:
			if err := c.collect(); err != nil {
				c.log.Errorf("collecting: %v", err)
			}
		case <-cleanupTicker.C:
			c.cleanup()
		}
	}
}

func (c *Collector) GetRawNetworkMetricsHandler(w http.ResponseWriter, req *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]*pb.RawNetworkMetric, 0, len(c.podMetrics))
	for _, m := range c.podMetrics {
		items = append(items, m.RawNetworkMetric)
	}

	batch := &pb.RawNetworkMetricBatch{Items: items}
	batchBytes, err := proto.Marshal(batch)
	if err != nil {
		c.log.Errorf("marshal batch: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(batchBytes); err != nil {
		c.log.Errorf("write batch: %v", err)
		return
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

	c.mu.Lock()
	defer c.mu.Unlock()

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
			pm.TxBytes += int64(txBytes)
			pm.TxPackets += int64(txPackets)
			pm.RxBytes += int64(rxBytes)
			pm.RxPackets += int64(rxPackets)
			if conn.Lifetime.After(pm.lifetime) {
				pm.lifetime = conn.Lifetime
			}
		} else {
			c.podMetrics[groupKey] = &rawNetworkMetric{
				RawNetworkMetric: &pb.RawNetworkMetric{
					SrcIp:     toIPint32(conn.Src.IP()),
					DstIp:     toIPint32(conn.Dst.IP()),
					TxBytes:   int64(conn.TxBytes),
					TxPackets: int64(conn.TxPackets),
					RxBytes:   int64(conn.RxBytes),
					RxPackets: int64(conn.RxPackets),
					Proto:     int32(conn.Proto),
				},
				lifetime: conn.Lifetime,
			}
		}
	}

	c.log.Debugf("collection done in %s, pods=%d, conntrack=%d, conntrack_cache=%d", time.Since(start), len(pods), len(conns), len(c.entriesCache))
	return nil
}

func (c *Collector) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	start := c.currentTimeGetter().UTC()
	now := start
	deletedEntriesCount := 0
	deletedPodMetricsCount := 0

	for key, e := range c.entriesCache {
		if now.After(e.Lifetime) {
			delete(c.entriesCache, key)
			deletedEntriesCount++
		}
	}

	for key, m := range c.podMetrics {
		if now.After(m.lifetime) {
			delete(c.podMetrics, key)
			deletedPodMetricsCount++
		}
	}

	c.log.Infof("cleanup done in %s, deleted_conntrack=%d, deleted_pod_metrics=%d", time.Since(start), deletedEntriesCount, deletedPodMetricsCount)
}

func (c *Collector) getNodePods() ([]*corev1.Pod, error) {
	pods, err := c.podsWatcher.Get(c.cfg.NodeName)
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

func toIPint32(ip netaddr.IP) int32 {
	b := ip.As4()
	return int32(binary.BigEndian.Uint32([]byte{b[0], b[1], b[2], b[3]}))
}
