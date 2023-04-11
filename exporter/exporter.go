package exporter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"inet.af/netaddr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	"github.com/castai/egressd/exporter/config"
	"github.com/castai/egressd/exporter/sinks"
	"github.com/castai/egressd/kube"
	"github.com/castai/egressd/pb"
)

const (
	collectorsConcurrentFetch = 20
	sinkConcurrentPush        = 5
)

func New(
	log logrus.FieldLogger,
	cfg config.Config,
	kubeWatcher kubeWatcher,
	kubeClient kubernetes.Interface,
	sinks []sinks.Sink,
) *Exporter {
	return &Exporter{
		cfg:         cfg,
		log:         log,
		kubeWatcher: kubeWatcher,
		kubeClient:  kubeClient,
		httpClient:  newHTTPClient(),
		sinks:       sinks,
	}
}

type Exporter struct {
	cfg         config.Config
	log         logrus.FieldLogger
	kubeWatcher kubeWatcher
	kubeClient  kubernetes.Interface
	httpClient  *http.Client
	sinks       []sinks.Sink
}

func (e *Exporter) Start(ctx context.Context) error {
	exportTicker := time.NewTicker(e.cfg.ExportInterval)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-exportTicker.C:
			if err := e.export(ctx); err != nil {
				e.log.Errorf("export: %v", err)
			}
		}
	}
}

func (e *Exporter) export(ctx context.Context) error {
	start := time.Now()
	e.log.Info("running export")
	defer func() {
		e.log.Infof("export done in %s", time.Since(start))
	}()

	// Find egressd collector pods.
	selector := labels.SelectorFromSet(labels.Set{
		"app.kubernetes.io/component": "egressd-collector",
	})
	collectorPodsList, err := e.kubeClient.CoreV1().Pods(e.cfg.PodNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		return err
	}

	collectorsCount := len(collectorPodsList.Items)

	if collectorsCount == 0 {
		e.log.Warnf("no active collector pods found")
		return nil
	}

	// Fetch network metrics from all collectors.
	e.log.Debugf("fetching network metrics from %d collector(s)", collectorsCount)

	pulledBatch := make(chan *pb.RawNetworkMetricBatch, collectorsCount)

	var fetchGroup errgroup.Group
	fetchGroup.SetLimit(collectorsConcurrentFetch)
	for _, pod := range collectorPodsList.Items {
		pod := pod

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fetchGroup.Go(func() (err error) {
			defer func() {
				if err != nil {
					pulledBatch <- &pb.RawNetworkMetricBatch{}
				}
			}()
			url := getCollectorPodMetricsServerURL(pod)
			batch, err := e.fetchRawNetworkMetricsBatch(ctx, url)
			if err != nil {
				e.log.Errorf("fetching metrics from collector, url=%s: %v", url, err)
				return nil
			}
			if len(batch.Items) == 0 {
				e.log.Warnf("no metrics found in collector %q", pod.Name)
				return nil
			}
			pulledBatch <- batch
			return nil
		})
	}
	if err := fetchGroup.Wait(); err != nil {
		return err
	}

	// Aggregate raw metrics into pod metrics.
	var podsMetrics []*pb.PodNetworkMetric
	for i := 0; i < collectorsCount; i++ {
		batch := <-pulledBatch
		for _, rawMetrics := range batch.Items {
			podMetrics, err := e.buildPodNetworkMetric(rawMetrics)
			if err != nil {
				e.log.Errorf("init pod network metrics: %v", err)
				continue
			}
			podsMetrics = append(podsMetrics, podMetrics)
		}
	}

	// Sink metrics to destinations.
	batch := &pb.PodNetworkMetricBatch{Items: podsMetrics}
	var sinkGroup errgroup.Group
	sinkGroup.SetLimit(sinkConcurrentPush)
	e.log.Debugf("sink to %d destination(s), pod_metrics=%d", len(e.sinks), len(podsMetrics))
	for _, s := range e.sinks {
		s := s
		sinkGroup.Go(func() error {
			if err := s.Push(ctx, batch); err != nil {
				e.log.Errorf("sink push: %v", err)
			}
			return nil
		})
	}
	if err := sinkGroup.Wait(); err != nil {
		return err
	}
	return err
}

func (e *Exporter) buildPodNetworkMetric(conn *pb.RawNetworkMetric) (*pb.PodNetworkMetric, error) {
	srcIP := parseIP(conn.SrcIp)
	pod, err := e.kubeWatcher.GetPodByIP(srcIP.String())
	if err != nil {
		return nil, fmt.Errorf("getting pod by ip %q: %w", srcIP.String(), err)
	}
	dstIP := parseIP(conn.DstIp)
	metric := pb.PodNetworkMetric{
		SrcIp:        conn.SrcIp,
		SrcPod:       pod.Name,
		SrcNamespace: pod.Namespace,
		SrcNode:      pod.Spec.NodeName,
		DstIp:        conn.DstIp,
		TxBytes:      conn.TxBytes,
		TxPackets:    conn.TxPackets,
		RxBytes:      conn.RxBytes,
		RxPackets:    conn.RxPackets,
		Proto:        conn.Proto,
	}

	srcNode, err := e.kubeWatcher.GetNodeByName(pod.Spec.NodeName)
	if err != nil && !errors.Is(err, kube.ErrNotFound) {
		return nil, err
	}
	if srcNode != nil {
		metric.SrcZone = getNodeZone(srcNode)
	}

	// Try to find destination pod and node info.
	if dstIP.IsPrivate() {
		dstIPStr := dstIP.String()
		// First try finding destination pod by ip.
		dstPod, err := e.kubeWatcher.GetPodByIP(dstIPStr)
		if err != nil && !errors.Is(err, kube.ErrNotFound) && !errors.Is(err, kube.ErrToManyObjects) {
			return nil, err
		}
		if dstPod != nil {
			metric.DstPod = dstPod.Name
			metric.DstNamespace = dstPod.Namespace

			// Also find destination node by name.
			dstNode, err := e.kubeWatcher.GetNodeByName(dstPod.Spec.NodeName)
			if err != nil && !errors.Is(err, kube.ErrNotFound) {
				return nil, err
			}
			if dstNode != nil {
				metric.DstNode = dstNode.Name
				metric.DstZone = getNodeZone(dstNode)
			}
		} else {
			// No destination pod found. But at least we can try finding destination node.
			dstNode, err := e.kubeWatcher.GetNodeByIP(dstIPStr)
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

func (e *Exporter) fetchRawNetworkMetricsBatch(ctx context.Context, url string) (*pb.RawNetworkMetricBatch, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v1/raw-network-metrics", url), nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if st := resp.StatusCode; st != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetching metrics from %q, status=%d: %s", url, st, string(body))
	}

	pbBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var batch pb.RawNetworkMetricBatch
	if err := proto.Unmarshal(pbBytes, &batch); err != nil {
		return nil, err
	}
	return &batch, nil
}

func getNodeZone(node *corev1.Node) string {
	return node.Labels["topology.kubernetes.io/zone"]
}

func parseIP(v int32) netaddr.IP {
	return netaddr.IPFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

func newHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}
	return &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

func getCollectorPodMetricsServerURL(pod corev1.Pod) string {
	ip := pod.Status.PodIP
	for _, cont := range pod.Spec.Containers {
		for _, port := range cont.Ports {
			if port.Name == "http-server" {
				return fmt.Sprintf("http://%s:%d", ip, port.ContainerPort)
			}
		}
	}
	return ""
}

type kubeWatcher interface {
	GetPodsByNode(nodeName string) ([]*corev1.Pod, error)
	GetPodByIP(ip string) (*corev1.Pod, error)
	GetNodeByName(name string) (*corev1.Node, error)
	GetNodeByIP(ip string) (*corev1.Node, error)
}
