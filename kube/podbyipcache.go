package kube

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

func NewPodByIPCache(informer cache.SharedIndexInformer, log logrus.FieldLogger) *PodByIPCache {
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

	c := &PodByIPCache{
		indexer:         indexer,
		deletedPodsMu:   sync.Mutex{},
		deletedPods:     map[types.UID]*corev1.Pod{},
		log:             log,
		cleanupInterval: 1 * time.Minute,
		// there are cases when connections might be kept in conntrack for 120 seconds
		// https://www.kernel.org/doc/Documentation/networking/nf_conntrack-sysctl.txt
		podTimeout: 2 * time.Minute,
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

type PodByIPCache struct {
	indexer cache.Indexer
	// sorted by DeletionTimestamp in ascending order
	cleanupInterval time.Duration
	podTimeout      time.Duration
	deletedPodsMu   sync.Mutex
	deletedPods     map[types.UID]*corev1.Pod
	log             logrus.FieldLogger
}

func (c *PodByIPCache) Run(ctx context.Context) {
	c.runCleanupExpiredLoop(ctx)
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
		// Add pod to delete list if
		if earliestPod.Status.Phase != corev1.PodPending && earliestPod.Status.Phase != corev1.PodRunning {
			if err := c.indexer.Delete(earliestPod); err != nil {
				return nil, err
			}
			c.deletedPodsMu.Lock()
			c.deletedPods[earliestPod.UID] = earliestPod
			c.deletedPodsMu.Unlock()
		}
		return earliestPod, nil
	}

	return nil, ErrNotFound
}

func (c *PodByIPCache) onDelete(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	c.deletedPodsMu.Lock()
	defer c.deletedPodsMu.Unlock()

	c.deletedPods[pod.UID] = pod
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

func (c *PodByIPCache) runCleanupExpiredLoop(ctx context.Context) {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			func() {
				c.deletedPodsMu.Lock()
				defer c.deletedPodsMu.Unlock()

				now := time.Now()
				for uid, pod := range c.deletedPods {
					// deletedPods should be sorted by DeletionTimeout in ascending order
					// as soon as we observe a pod that is not yet timed out it is safe to break
					// we shouldn't have pods with no deletion timestamp in this slice
					if pod.DeletionTimestamp != nil && now.Sub(pod.DeletionTimestamp.Time) <= c.podTimeout {
						break
					}
					err := c.indexer.Delete(pod)
					if err != nil {
						c.log.Errorf("cannot remove pod from cache %s", err)
					}
					delete(c.deletedPods, uid)
				}
			}()

		case <-ctx.Done():
			return
		}
	}
}
