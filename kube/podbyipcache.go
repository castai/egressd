package kube

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

var (
	// there are cases when connections might be kept in conntrack for 120 seconds
	// https://www.kernel.org/doc/Documentation/networking/nf_conntrack-sysctl.txt
	podTimeout = 2 * time.Minute
)

type PodByIPCache struct {
	indexer cache.Indexer
	// sorted by DeletionTimestamp in ascending order
	deletedPods []*corev1.Pod
	mu          sync.Mutex
	log         logrus.FieldLogger
}

func (c *PodByIPCache) Get(key string) (*corev1.Pod, error) {
	items, err := c.indexer.ByIndex(podIPIdx, key)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}

	var earliestPod *corev1.Pod
	for _, item := range items {
		pod, ok := item.(*corev1.Pod)
		if !ok {
			continue
		}

		if earliestPod == nil || pod.CreationTimestamp.Before(&earliestPod.CreationTimestamp) {
			earliestPod = pod
		}
	}

	if earliestPod != nil {
		return earliestPod, nil
	}

	return nil, ErrNotFound
}

func (c *PodByIPCache) onDelete(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deletedPods = append(c.deletedPods, pod)
}

func (c *PodByIPCache) onAddOrUpdate(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	err := c.indexer.Add(pod)
	if err != nil {
		c.log.Errorf("cannot add pod to cache %s", err)
	}
}

func (c *PodByIPCache) cleanupExpired(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.mu.Lock()

			now := time.Now()
			var i int
			for i = 0; i < len(c.deletedPods); i++ {
				pod := c.deletedPods[i]
				// deletedPods should be sorted by DeletionTimeout in ascending order
				// as soon as we observe a pod that is not yet timed out it is safe to break
				// we shouldn't have pods with no deletion timestamp in this slice
				if pod.DeletionTimestamp != nil && now.Sub(pod.DeletionTimestamp.Time) <= podTimeout {
					break
				}
				err := c.indexer.Delete(pod)
				if err != nil {
					c.log.Errorf("cannot remove pod from cache %s", err)
				}
			}
			c.deletedPods = c.deletedPods[i:]

			c.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func NewPodByIPCache(ctx context.Context, informer cache.SharedIndexInformer, log logrus.FieldLogger) *PodByIPCache {
	if err := informer.SetTransform(defaultTransformFunc); err != nil {
		panic(err)
	}

	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		podIPIdx: func(obj interface{}) ([]string, error) {
			switch t := obj.(type) {
			case *corev1.Pod:
				return []string{t.Status.PodIP}, nil
			}
			return nil, fmt.Errorf("expected pod, got unknown type %T", obj)
		},
	})

	c := &PodByIPCache{indexer: indexer, mu: sync.Mutex{}, log: log}

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

	go c.cleanupExpired(ctx)

	return c
}
