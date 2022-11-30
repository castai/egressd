package collector

import (
	"context"
	"errors"
	"hash/maphash"
	"strings"
	"time"

	"github.com/castai/egressd/conntrack"
	"github.com/castai/egressd/kube"
	"github.com/sirupsen/logrus"
	"inet.af/netaddr"
	corev1 "k8s.io/api/core/v1"
)

func New(cfg Config, log logrus.FieldLogger, kubeWatcher kube.Watcher, conntracker conntrack.Client) *Collector {
	excludeNsMap := map[string]struct{}{}
	if cfg.ExcludeNamespaces != "" {
		nsList := strings.Split(cfg.ExcludeNamespaces, ",")
		for _, ns := range nsList {
			excludeNsMap[ns] = struct{}{}
		}
	}
	return &Collector{
		cfg:          cfg,
		log:          log,
		kubeWatcher:  kubeWatcher,
		conntracker:  conntracker,
		metricsChan:  make(chan PodNetworkMetric, 10000),
		excludeNsMap: excludeNsMap,
	}
}

type Config struct {
	Interval          time.Duration
	NodeName          string
	ExcludeNamespaces string
}

type Collector struct {
	cfg          Config
	log          logrus.FieldLogger
	kubeWatcher  kube.Watcher
	conntracker  conntrack.Client
	excludeNsMap map[string]struct{}

	//mu          sync.RWMutex
	//metrics     []PodNetworkMetric
	metricsChan chan PodNetworkMetric
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
			a.log.Debug("collecting pod network metrics")
			if err := a.run(); err != nil {
				a.log.Errorf("collect error: %v", err)
			} else {
				a.log.Debug("collection done")
			}
		}
	}
}

func (a *Collector) run() error {
	pods, err := a.kubeWatcher.GetPodsByNode(a.cfg.NodeName)
	if err != nil {
		return err
	}

	conns, err := a.conntracker.ListEntries()
	if err != nil {
		return err
	}

	ts := time.Now().UnixMilli() // Generate timestamp which is added for each metric during this cycle.
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

		podConns, found := conns[netaddr.MustParseIP(podIP)]
		if !found {
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
				a.log.Warning("dropping metric event, channel is full")
			}
		}
	}

	return nil
}

func (a *Collector) aggregatePodNetworkMetrics(pod *corev1.Pod, podConns []conntrack.Entry, ts int64) ([]PodNetworkMetric, error) {
	grouped, err := groupConns(podConns)
	if err != nil {
		return nil, err
	}
	res := make([]PodNetworkMetric, 0, len(grouped))
	for _, conn := range grouped {
		metric := PodNetworkMetric{
			SrcIP:        conn.srcIP.String(),
			DstIP:        conn.dstIP.String(),
			SrcPod:       pod.Name,
			SrcNamespace: pod.Namespace,
			SrcNode:      pod.Spec.NodeName,
			SrcZone:      "",
			DstPod:       "",
			DstNamespace: "",
			DstNode:      "",
			DstZone:      "",
			TxBytes:      conn.txBytes,
			TxPackets:    conn.txPackets,
			Proto:        conntrack.ProtoString(conn.proto),
			Ts:           uint64(ts),
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

type groupedConn struct {
	srcIP     netaddr.IP
	dstIP     netaddr.IP
	proto     uint8
	txBytes   uint64
	txPackets uint64
}

func groupConns(conns []conntrack.Entry) (map[uint64]*groupedConn, error) {
	grouped := make(map[uint64]*groupedConn)
	for _, conn := range conns {
		// TODO: Fix this logic. For Cilium this is OK. For linux nf we actually want reply.
		if conn.Reply {
			continue
		}
		key, err := connKey(&conn)
		if err != nil {
			return nil, err
		}
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
	}
	return grouped, nil
}

var connHash maphash.Hash

func connKey(conn *conntrack.Entry) (uint64, error) {
	srcIP := conn.Src.IP().As4()
	if _, err := connHash.Write(srcIP[:]); err != nil {
		return 0, err
	}
	dstIP := conn.Dst.IP().As4()
	if _, err := connHash.Write(dstIP[:]); err != nil {
		return 0, err
	}
	if err := connHash.WriteByte(conn.Proto); err != nil {
		return 0, err
	}
	res := connHash.Sum64()
	connHash.Reset()
	return res, nil
}

func getNodeZone(node *corev1.Node) string {
	return node.Labels["topology.kubernetes.io/zone"]
}
