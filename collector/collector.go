package collector

import (
	"context"
	"encoding/binary"
	"errors"
	"hash/maphash"
	"strings"
	"time"

	"github.com/castai/egressd/conntrack"
	"github.com/castai/egressd/kube"
	"github.com/castai/egressd/metrics"
	"github.com/sirupsen/logrus"
	"inet.af/netaddr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func New(
	cfg Config,
	log logrus.FieldLogger,
	kubeWatcher kube.Watcher,
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
	processedEntriesCache := make(map[uint64]*conntrack.Entry)
	return &Collector{
		cfg:                   cfg,
		log:                   log,
		kubeWatcher:           kubeWatcher,
		conntracker:           conntracker,
		processedEntriesCache: processedEntriesCache,
		metricsChan:           make(chan PodNetworkMetric, 10000),
		excludeNsMap:          excludeNsMap,
		currentTimeGetter:     currentTimeGetter,
	}
}

func CurrentTimeGetter() func() time.Time {
	return func() time.Time {
		return time.Now()
	}
}

type Config struct {
	Interval          time.Duration
	NodeName          string
	ExcludeNamespaces string
	CacheItems        int
}

type Collector struct {
	cfg                   Config
	log                   logrus.FieldLogger
	kubeWatcher           kube.Watcher
	conntracker           conntrack.Client
	processedEntriesCache map[uint64]*conntrack.Entry
	excludeNsMap          map[string]struct{}
	metricsChan           chan PodNetworkMetric
	currentTimeGetter     func() time.Time
}

func (a *Collector) GetMetricsChan() <-chan PodNetworkMetric {
	return a.metricsChan
}

func (a *Collector) Start(ctx context.Context) error {
	ticker := time.NewTicker(a.cfg.Interval)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			start := time.Now()
			a.log.Debug("collecting pod network metrics")
			if err := a.run(); err != nil {
				a.log.Errorf("collect error: %v", err)
			} else {
				a.log.Debugf("collection done in %s", time.Since(start))
			}
		}
	}
}

func (a *Collector) run() error {
	pods, err := a.kubeWatcher.GetPodsByNode(a.cfg.NodeName)
	if err != nil {
		return err
	}

	// Filter pods for tracking.
	ips := make(map[netaddr.IP]struct{}, 0)
	var filteredPods []*corev1.Pod
	for _, pod := range pods {
		podIP := pod.Status.PodIP
		if podIP == "" {
			continue
		}
		// Don't track host network pods since we don't have enough info in conntrack.
		if pod.Spec.HostNetwork {
			continue
		}
		if _, found := a.excludeNsMap[pod.Namespace]; found {
			continue
		}
		ips[netaddr.MustParseIP(pod.Status.PodIP)] = struct{}{}
		filteredPods = append(filteredPods, pod)
	}

	// Add special host network pod to handle host network traffic.
	srcNode, err := a.kubeWatcher.GetNodeByName(a.cfg.NodeName)
	if err != nil && !errors.Is(err, kube.ErrNotFound) {
		return err
	}
	if srcNode != nil {
		nodeIP := getNodePrivateIP(srcNode)
		podIP := nodeIP.String()
		if podIP != "" {
			hostNetworkPod := &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Name:      "host-network",
					Namespace: "host-network",
				},
				Spec: corev1.PodSpec{
					NodeName: a.cfg.NodeName,
				},
				Status: corev1.PodStatus{
					PodIP: podIP,
				},
			}
			filteredPods = append(filteredPods, hostNetworkPod)
			ips[nodeIP] = struct{}{}
		}
	}

	conns, err := a.conntracker.ListEntries(conntrack.FilterByIPs(ips))
	if err != nil {
		return err
	}
	metrics.SetConntrackActiveEntriesCount(float64(len(conns)))

	ts := a.currentTimeGetter().UTC().UnixMilli() // Generate timestamp which is added for each metric during this cycle.
	records := make([]conntrack.Entry, 0)
	for _, pod := range filteredPods {
		podConns := filterConnEntriesByIP(conns, netaddr.MustParseIP(pod.Status.PodIP))
		if len(podConns) == 0 {
			continue
		}
		podMetrics, err := a.aggregatePodNetworkMetrics(pod, podConns, ts)
		if err != nil {
			return err
		}
		a.log.Debugf("pod=%s, conns=%d, metrics=%d", pod.Name, len(podConns), len(podMetrics))
		for _, metric := range podMetrics {
			select {
			case a.metricsChan <- metric:
			default:
				metrics.IncDroppedEvents()
				a.log.Warning("dropping metric event, channel is full")
			}
		}
		records = append(records, podConns...)
	}

	a.markProcessedEntries(records)

	return nil
}

func (a *Collector) markProcessedEntries(entries []conntrack.Entry) {
	newCache := make(map[uint64]*conntrack.Entry)
	for _, e := range entries {
		e := e
		// TODO: Cache key is now calculated 2 times. Precalculate once during conntrack records fetch.
		newCache[entryKey(&e)] = &e
	}
	a.log.Infof("updating conntrack records, old length: %d, new length: %d", len(a.processedEntriesCache), len(newCache))
	a.processedEntriesCache = newCache
}

func (a *Collector) aggregatePodNetworkMetrics(pod *corev1.Pod, podConns []conntrack.Entry, ts int64) ([]PodNetworkMetric, error) {
	changedConns := make([]conntrack.Entry, 0)
	for _, conn := range podConns {
		conn := conn
		hash := entryKey(&conn)
		prev, found := a.processedEntriesCache[hash]
		if found {
			if conn.TxBytes != prev.TxBytes || conn.RxBytes != prev.RxBytes {
				conn.TxBytes = conn.TxBytes - prev.TxBytes
				conn.RxBytes = conn.RxBytes - prev.RxBytes
				conn.TxPackets = conn.TxPackets - prev.TxPackets
				conn.RxPackets = conn.RxPackets - prev.RxPackets
				changedConns = append(changedConns, conn)
			}
			continue
		}
		changedConns = append(changedConns, conn)
	}

	grouped := groupConns(changedConns)
	res := make([]PodNetworkMetric, 0, len(grouped))
	for _, conn := range grouped {
		metric := PodNetworkMetric{
			SrcIP:        conn.srcIP.String(),
			SrcPod:       pod.Name,
			SrcNamespace: pod.Namespace,
			SrcNode:      pod.Spec.NodeName,
			SrcZone:      "",
			DstIP:        conn.dstIP.String(),
			DstIPType:    ipType(conn.dstIP),
			DstPod:       "",
			DstNamespace: "",
			DstNode:      "",
			DstZone:      "",
			TxBytes:      conn.txBytes,
			TxPackets:    conn.txPackets,
			RxBytes:      conn.rxBytes,
			RxPackets:    conn.rxPackets,
			Proto:        conntrack.ProtoString(conn.proto),
			TS:           uint64(ts),
		}

		srcNode, err := a.kubeWatcher.GetNodeByName(pod.Spec.NodeName)
		if err != nil && !errors.Is(err, kube.ErrNotFound) {
			return nil, err
		}
		if srcNode != nil {
			metric.SrcZone = getNodeZone(srcNode)
		}

		// Try to find destination pod and node info.
		if conn.dstIP.IsPrivate() {
			dstIP := conn.dstIP.String()
			// First try finding destination pod by ip.
			dstPod, err := a.kubeWatcher.GetPodByIP(dstIP)
			if err != nil && !errors.Is(err, kube.ErrNotFound) && !errors.Is(err, kube.ErrToManyObjects) {
				return nil, err
			}
			if dstPod != nil {
				metric.DstPod = dstPod.Name
				metric.DstNamespace = dstPod.Namespace

				// Also find destination node by name.
				dstNode, err := a.kubeWatcher.GetNodeByName(dstPod.Spec.NodeName)
				if err != nil && !errors.Is(err, kube.ErrNotFound) {
					return nil, err
				}
				if dstNode != nil {
					metric.DstNode = dstNode.Name
					metric.DstZone = getNodeZone(dstNode)
				}
			} else {
				// No destination pod found. But at least we can try finding destination node.
				dstNode, err := a.kubeWatcher.GetNodeByIP(dstIP)
				if err != nil && !errors.Is(err, kube.ErrNotFound) {
					return nil, err
				}
				if dstNode != nil {
					metric.DstNode = dstNode.Name
					metric.DstZone = getNodeZone(dstNode)
				}
			}
		}

		res = append(res, metric)
	}
	return res, nil
}

func filterConnEntriesByIP(entries []conntrack.Entry, ip netaddr.IP) []conntrack.Entry {
	var res []conntrack.Entry
	for _, e := range entries {
		if e.Src.IP() == ip {
			res = append(res, e)
		} else if e.Dst.IP() == ip {
			// Swap entry since we always want source pointing to pod ip.
			res = append(res, conntrack.Entry{
				Src:       e.Dst,
				Dst:       e.Src,
				TxBytes:   e.RxBytes,
				TxPackets: e.RxPackets,
				RxBytes:   e.TxBytes,
				RxPackets: e.RxPackets,
				Proto:     e.Proto,
			})
		}
	}
	return res
}

type groupedConn struct {
	srcIP     netaddr.IP
	dstIP     netaddr.IP
	proto     uint8
	txBytes   uint64
	txPackets uint64
	rxBytes   uint64
	rxPackets uint64
}

func groupConns(conns []conntrack.Entry) map[uint64]*groupedConn {
	grouped := make(map[uint64]*groupedConn)
	for _, conn := range conns {
		conn := conn
		key := connGroupKey(&conn)
		group, found := grouped[key]
		if !found {
			group = &groupedConn{
				srcIP: conn.Src.IP(),
				dstIP: conn.Dst.IP(),
				proto: conn.Proto,
			}
			grouped[key] = group
		}
		group.txBytes += conn.TxBytes
		group.txPackets += conn.TxPackets
		group.rxBytes += conn.RxBytes
		group.rxPackets += conn.RxPackets
	}
	return grouped
}

var connHash maphash.Hash

func connGroupKey(conn *conntrack.Entry) uint64 {
	srcIP := conn.Src.IP().As4()
	_, _ = connHash.Write(srcIP[:])
	dstIP := conn.Dst.IP().As4()
	_, _ = connHash.Write(dstIP[:])
	_ = connHash.WriteByte(conn.Proto)
	res := connHash.Sum64()
	connHash.Reset()
	return res
}

var entryHash maphash.Hash

func entryKey(conn *conntrack.Entry) uint64 {
	srcIP := conn.Src.IP().As4()
	_, _ = entryHash.Write(srcIP[:])
	var srcPort [2]byte
	binary.LittleEndian.PutUint16(srcPort[:], conn.Src.Port())
	_, _ = entryHash.Write(srcPort[:])

	dstIP := conn.Dst.IP().As4()
	_, _ = entryHash.Write(dstIP[:])
	var dstPort [2]byte
	binary.LittleEndian.PutUint16(dstPort[:], conn.Dst.Port())
	_, _ = entryHash.Write(dstPort[:])

	_ = entryHash.WriteByte(conn.Proto)
	res := entryHash.Sum64()

	entryHash.Reset()
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

func getNodePrivateIP(n *corev1.Node) netaddr.IP {
	for _, addr := range n.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return netaddr.MustParseIP(addr.Address)
		}
	}
	return netaddr.IP{}
}
