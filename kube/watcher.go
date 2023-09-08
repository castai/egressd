package kube

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	podIPIdx    = "pods-by-ip"
	podNodeIdx  = "pods-by-node"
	nodeNameIDx = "nodes-by-name"
	nodeIPIdx   = "nodes-by-ip"
)

var (
	ErrNotFound      = errors.New("object not found")
	ErrToManyObjects = errors.New("too many objects")

	exitedPodTimeout = 1 * time.Minute
)

func NewPodsByNodeCache(informer cache.SharedIndexInformer) *PodsByNodeCache {
	if err := informer.SetTransform(defaultTransformFunc); err != nil {
		panic(err)
	}
	if err := informer.AddIndexers(map[string]cache.IndexFunc{
		podNodeIdx: func(obj interface{}) ([]string, error) {
			switch t := obj.(type) {
			case *corev1.Pod:
				if t.Status.Phase == corev1.PodRunning {
					return []string{t.Spec.NodeName}, nil
				}
				return []string{}, nil
			}
			return nil, fmt.Errorf("expected pod, got unknown type %T", obj)
		},
	}); err != nil {
		panic(err)
	}
	return &PodsByNodeCache{informer: informer}
}

type PodsByNodeCache struct {
	informer cache.SharedIndexInformer
}

func (p *PodsByNodeCache) Get(nodeName string) ([]*corev1.Pod, error) {
	pods, err := p.informer.GetIndexer().ByIndex(podNodeIdx, nodeName)
	if err != nil {
		return nil, err
	}
	res := make([]*corev1.Pod, len(pods))
	for i, pod := range pods {
		res[i] = pod.(*corev1.Pod)
	}
	return res, nil
}

func NewPodByIPCache(informer cache.SharedIndexInformer) *PodByIPCache {
	if err := informer.SetTransform(defaultTransformFunc); err != nil {
		panic(err)
	}
	if err := informer.AddIndexers(map[string]cache.IndexFunc{
		podIPIdx: func(obj interface{}) ([]string, error) {
			switch t := obj.(type) {
			case *corev1.Pod:
				if t.Status.Phase == corev1.PodRunning {
					return []string{t.Status.PodIP}, nil
				}
				if t.Status.Phase == corev1.PodSucceeded || t.Status.Phase == corev1.PodFailed {
					if podExitedBeforeTimeout(t) {
						return []string{t.Status.PodIP}, nil
					}
				}
				return []string{}, nil
			}
			return nil, fmt.Errorf("expected pod, got unknown type %T", obj)
		},
	}); err != nil {
		panic(err)
	}
	return &PodByIPCache{informer: informer}
}

func podExitedBeforeTimeout(t *corev1.Pod) bool {
	podReady, found := lo.Find(t.Status.Conditions, func(c corev1.PodCondition) bool {
		return c.Type == corev1.PodReady
	})
	deadLine := time.Now().UTC().Add(-exitedPodTimeout)
	return found && podReady.LastTransitionTime.UTC().After(deadLine)
}

type PodByIPCache struct {
	informer cache.SharedIndexInformer
}

func (p *PodByIPCache) Get(ip string) (*corev1.Pod, error) {
	items, err := p.informer.GetIndexer().ByIndex(podIPIdx, ip)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	pods := lo.FilterMap(items, func(item interface{}, i int) (*corev1.Pod, bool) {
		pod, ok := item.(*corev1.Pod)
		if !ok {
			return nil, false
		}
		return pod, true
	})
	sort.SliceStable(pods, func(i, j int) bool {
		return pods[i].CreationTimestamp.Before(&pods[j].CreationTimestamp)
	})
	for i := range pods {
		if pod := pods[i]; pod != nil {
			return pod, nil
		}
	}
	return nil, ErrNotFound
}

func NewNodeByNameCache(informer cache.SharedIndexInformer) *NodeByNameCache {
	if err := informer.SetTransform(defaultTransformFunc); err != nil {
		panic(err)
	}
	if err := informer.AddIndexers(map[string]cache.IndexFunc{
		nodeNameIDx: func(obj interface{}) ([]string, error) {
			switch t := obj.(type) {
			case *corev1.Node:
				return []string{t.Name}, nil
			}
			return nil, fmt.Errorf("expected node, got unknown type %T", obj)
		},
	}); err != nil {
		panic(err)
	}
	return &NodeByNameCache{informer: informer}
}

type NodeByNameCache struct {
	informer cache.SharedIndexInformer
}

func (n *NodeByNameCache) Get(nodeName string) (*corev1.Node, error) {
	nodes, err := n.informer.GetIndexer().ByIndex(nodeNameIDx, nodeName)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrNotFound
	}
	if len(nodes) > 1 {
		return nil, fmt.Errorf("expected single node for name %s, got %d", nodeName, len(nodes))
	}
	node := nodes[0]
	return node.(*corev1.Node), nil
}

func NewNodeByIPCache(informer cache.SharedIndexInformer) *NodeByIPCache {
	if err := informer.SetTransform(defaultTransformFunc); err != nil {
		panic(err)
	}
	if err := informer.AddIndexers(map[string]cache.IndexFunc{
		nodeIPIdx: func(obj interface{}) ([]string, error) {
			switch t := obj.(type) {
			case *corev1.Node:
				for _, addr := range t.Status.Addresses {
					if addr.Type == corev1.NodeInternalIP {
						return []string{addr.Address}, nil
					}
				}
				return []string{}, nil
			}
			return nil, fmt.Errorf("expected node, got unknown type %T", obj)
		},
	}); err != nil {
		panic(err)
	}
	return &NodeByIPCache{informer: informer}
}

type NodeByIPCache struct {
	informer cache.SharedIndexInformer
}

func (n *NodeByIPCache) Get(nodeIP string) (*corev1.Node, error) {
	nodes, err := n.informer.GetIndexer().ByIndex(nodeIPIdx, nodeIP)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrNotFound
	}
	if len(nodes) > 1 {
		return nil, fmt.Errorf("expected single node for ip %s, got %d", nodeIP, len(nodes))
	}
	node := nodes[0]
	return node.(*corev1.Node), nil
}

// defaultTransformFunc removes unused fields from kubernetes objects to reduce memory usage.
func defaultTransformFunc(obj interface{}) (interface{}, error) {
	switch t := obj.(type) {
	case *corev1.Pod:
		t.SetManagedFields(nil)
		t.Spec = corev1.PodSpec{
			NodeName:    t.Spec.NodeName,
			HostNetwork: t.Spec.HostNetwork,
		}
		t.Status = corev1.PodStatus{
			PodIP:      t.Status.PodIP,
			Phase:      t.Status.Phase,
			Conditions: t.Status.Conditions,
		}
		t.SetLabels(nil)
		t.SetAnnotations(nil)
		return t, nil
	case *corev1.Node:
		t.SetManagedFields(nil)
		t.Spec = corev1.NodeSpec{}
		t.Status = corev1.NodeStatus{
			Addresses: t.Status.Addresses,
		}
		return t, nil
	}
	return nil, fmt.Errorf("unknown type %T", obj)
}
