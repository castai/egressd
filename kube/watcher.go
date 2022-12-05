package kube

import (
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
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
)

type Watcher interface {
	GetPodsByNode(nodeName string) ([]*corev1.Pod, error)
	GetPodByIP(ip string) (*corev1.Pod, error)
	GetNodeByName(name string) (*corev1.Node, error)
	GetNodeByIP(ip string) (*corev1.Node, error)
}

func NewWatcher(k8sClient kubernetes.Interface) Watcher {
	factory := informers.NewSharedInformerFactoryWithOptions(k8sClient, 30*time.Second)

	podInformer := factory.Core().V1().Pods().Informer()
	if err := podInformer.SetTransform(transformFunc); err != nil {
		panic(err)
	}
	err := podInformer.AddIndexers(map[string]cache.IndexFunc{
		podIPIdx:   podIPIndexerFunc,
		podNodeIdx: podNodeIndexerFunc,
	})
	if err != nil {
		panic(err)
	}

	nodeInformer := factory.Core().V1().Nodes().Informer()
	if err := nodeInformer.SetTransform(transformFunc); err != nil {
		panic(err)
	}
	err = nodeInformer.AddIndexers(map[string]cache.IndexFunc{
		nodeNameIDx: nodeNameIndexerFunc,
		nodeIPIdx:   nodeIPIndexerFunc,
	})
	if err != nil {
		panic(err)
	}

	factory.Start(wait.NeverStop)
	factory.WaitForCacheSync(wait.NeverStop)
	return &kubernetesWatcher{
		podInformer:  podInformer,
		nodeInformer: nodeInformer,
	}
}

type kubernetesWatcher struct {
	podInformer  cache.SharedIndexInformer
	nodeInformer cache.SharedIndexInformer
}

func (k *kubernetesWatcher) GetPodsByNode(nodeName string) ([]*corev1.Pod, error) {
	pods, err := k.podInformer.GetIndexer().ByIndex(podNodeIdx, nodeName)
	if err != nil {
		return nil, err
	}
	res := make([]*corev1.Pod, len(pods))
	for i, pod := range pods {
		res[i] = pod.(*corev1.Pod)
	}
	return res, nil
}

func (k *kubernetesWatcher) GetPodByIP(ip string) (*corev1.Pod, error) {
	pods, err := k.podInformer.GetIndexer().ByIndex(podIPIdx, ip)
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		return nil, ErrNotFound
	}
	if len(pods) > 1 {
		return nil, ErrToManyObjects
	}
	pod := pods[0]
	return pod.(*corev1.Pod), nil
}

func (k *kubernetesWatcher) GetNodeByName(name string) (*corev1.Node, error) {
	nodes, err := k.nodeInformer.GetIndexer().ByIndex(nodeNameIDx, name)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrNotFound
	}
	if len(nodes) > 1 {
		return nil, fmt.Errorf("expected single node for name %s, got %d", name, len(nodes))
	}
	node := nodes[0]
	return node.(*corev1.Node), nil
}

func (k *kubernetesWatcher) GetNodeByIP(ip string) (*corev1.Node, error) {
	nodes, err := k.nodeInformer.GetIndexer().ByIndex(nodeIPIdx, ip)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrNotFound
	}
	if len(nodes) > 1 {
		return nil, ErrToManyObjects
	}
	node := nodes[0]
	return node.(*corev1.Node), nil
}

func podNodeIndexerFunc(obj interface{}) ([]string, error) {
	switch t := obj.(type) {
	case *corev1.Pod:
		return []string{t.Spec.NodeName}, nil
	}
	return nil, fmt.Errorf("expected pod, got unknown type %T", obj)
}

func podIPIndexerFunc(obj interface{}) ([]string, error) {
	switch t := obj.(type) {
	case *corev1.Pod:
		return []string{t.Status.PodIP}, nil
	}
	return nil, fmt.Errorf("expected pod, got unknown type %T", obj)
}

func nodeNameIndexerFunc(obj interface{}) ([]string, error) {
	switch t := obj.(type) {
	case *corev1.Node:
		return []string{t.Name}, nil
	}
	return nil, fmt.Errorf("expected node, got unknown type %T", obj)
}

func nodeIPIndexerFunc(obj interface{}) ([]string, error) {
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
}

// transformFunc removes unused fields from kubernetes objects to reduce memory usage.
func transformFunc(obj interface{}) (interface{}, error) {
	switch t := obj.(type) {
	case *corev1.Pod:
		t.SetManagedFields(nil)
		t.Spec = corev1.PodSpec{
			NodeName:    t.Spec.NodeName,
			HostNetwork: t.Spec.HostNetwork,
		}
		t.Status = corev1.PodStatus{
			PodIP: t.Status.PodIP,
		}
		t.SetLabels(nil)
		t.SetAnnotations(nil)
		return t, nil
	case *corev1.Node:
		t.SetManagedFields(nil)
		t.Spec = corev1.NodeSpec{}
		t.Status = corev1.NodeStatus{}
		return t, nil
	}
	return nil, fmt.Errorf("unknown type %T", obj)
}
