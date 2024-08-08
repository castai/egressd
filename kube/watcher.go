package kube

import (
	"errors"
	"fmt"
	"sync"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

const (
	podIPIdx    = "pods-by-ip"
	nodeNameIDx = "nodes-by-name"
	nodeIPIdx   = "nodes-by-ip"
)

var (
	ErrNotFound      = errors.New("object not found")
	ErrToManyObjects = errors.New("too many objects")
)

func NewRunningPodsCache(informer cache.SharedIndexInformer) *RunningPodsCache {
	if err := informer.SetTransform(defaultTransformFunc); err != nil {
		panic(err)
	}

	c := &RunningPodsCache{
		informer: informer,
		pods:     map[types.UID]*corev1.Pod{},
	}
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			c.onDelete(obj)
		},
		AddFunc: func(obj interface{}) {
			c.onAddOrUpdate(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			c.onAddOrUpdate(newObj)
		},
	})
	if err != nil {
		panic(err)
	}

	return c
}

type RunningPodsCache struct {
	informer cache.SharedIndexInformer

	mu   sync.Mutex
	pods map[types.UID]*corev1.Pod
}

func (p *RunningPodsCache) Get() ([]*corev1.Pod, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return lo.Values(p.pods), nil
}

func (p *RunningPodsCache) onDelete(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pods, pod.GetUID())
}

func (p *RunningPodsCache) onAddOrUpdate(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	if pod.Status.PodIP == "" || pod.Status.Phase != corev1.PodRunning {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.pods[pod.GetUID()] = pod
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
