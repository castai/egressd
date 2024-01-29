package collector

import (
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"hash/maphash"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"inet.af/netaddr"
	corev1 "k8s.io/api/core/v1"

	"github.com/castai/egressd/conntrack"
	"github.com/castai/egressd/dns"
	"github.com/castai/egressd/metrics"
	"github.com/castai/egressd/pb"
)

var (
	acceptEncoding  = http.CanonicalHeaderKey("Accept-Encoding")
	contentEncoding = http.CanonicalHeaderKey("Content-Encoding")
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
	// GroupPublicIPs will group all public destinations under single 0.0.0.0 IP.
	GroupPublicIPs bool
	// SendTrafficDelta used to determines if traffic should be sent as delta of 2 consecutive conntrack entries
	// or as the constantly growing counter value
	SendTrafficDelta bool
}

type podsWatcher interface {
	Get(nodeName string) ([]*corev1.Pod, error)
}

type rawNetworkMetric struct {
	*pb.RawNetworkMetric
	lifetime time.Time
}

type dnsRecorder interface{ Records() []*pb.IP2Domain }

func New(
	cfg Config,
	log logrus.FieldLogger,
	podsWatcher podsWatcher,
	conntracker conntrack.Client,
	ip2dns dnsRecorder,
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
		ip2dns:            ip2dns,
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
	ip2dns            dnsRecorder
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

	batch := &pb.RawNetworkMetricBatch{Items: items, Ip2Domain: c.ip2dns.Records()}
	batchBytes, err := proto.Marshal(batch)
	if err != nil {
		c.log.Errorf("marshal batch: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	enc := req.Header.Get(acceptEncoding)
	if strings.Contains(strings.ToLower(enc), "gzip") {
		if err := c.writeGzipBody(w, batchBytes); err != nil {
			c.log.Errorf("write batch %v", err)
			return
		}
	} else {
		if err := c.writePlainBody(w, batchBytes); err != nil {
			c.log.Errorf("write batch %v", err)
			return
		}
	}

	if c.cfg.SendTrafficDelta {
		// reset metric tx/rx values, so only delta numbers will be sent with the next batch
		for _, m := range c.podMetrics {
			m.RawNetworkMetric.TxBytes = 0
			m.RawNetworkMetric.RxBytes = 0
			m.RawNetworkMetric.TxPackets = 0
			m.RawNetworkMetric.RxPackets = 0

			newLifetime := time.Now().Add(30 * time.Second)
			// reset lifetime only if current lifetime is longer than 2 minutes from now
			if m.lifetime.After(newLifetime) {
				m.lifetime = newLifetime
			}
		}
	}
}

func (c *Collector) writeGzipBody(w http.ResponseWriter, body []byte) error {
	writer, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return fmt.Errorf("cannot create gzip writer: %w", err)
	}
	defer writer.Close()

	w.Header().Add(contentEncoding, "gzip")

	if _, err := writer.Write(body); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return err
	}

	return nil
}

func (c *Collector) writePlainBody(w http.ResponseWriter, body []byte) error {
	if _, err := w.Write(body); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return err
	}
	return nil
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
		if c.cfg.GroupPublicIPs && !conn.Dst.IP().IsPrivate() {
			conn.Dst = netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 0)
		}
		connKey := conntrackEntryKey(conn)
		txBytes := conn.TxBytes
		txPackets := conn.TxPackets
		rxBytes := conn.RxBytes
		rxPackets := conn.RxPackets

		if cachedConn, found := c.entriesCache[connKey]; found {
			// NOTE: REP-243: there is known issue that current tx/rx bytes could be lower than previously scrapped values,
			// so treat it as 0 delta to avoid random values for uint64
			txBytes = lo.Ternary(txBytes < cachedConn.TxBytes, 0, txBytes-cachedConn.TxBytes)
			rxBytes = lo.Ternary(rxBytes < cachedConn.RxBytes, 0, rxBytes-cachedConn.RxBytes)
			txPackets = lo.Ternary(txPackets < cachedConn.TxPackets, 0, txPackets-cachedConn.TxPackets)
			rxPackets = lo.Ternary(rxPackets < cachedConn.RxPackets, 0, rxPackets-cachedConn.RxPackets)
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
					SrcIp:      dns.ToIPint32(conn.Src.IP()),
					DstIp:      dns.ToIPint32(conn.Dst.IP()),
					TxBytes:    int64(conn.TxBytes),
					TxPackets:  int64(conn.TxPackets),
					RxBytes:    int64(conn.RxBytes),
					RxPackets:  int64(conn.RxPackets),
					Proto:      int32(conn.Proto),
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
