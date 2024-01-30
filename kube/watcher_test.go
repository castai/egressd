package kube

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWatcher(t *testing.T) {

	n1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "n1",
			CreationTimestamp: metav1.Now(),
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: "10.0.0.5",
				},
			},
		},
	}

	// regular running pod
	p1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "team1",
		},
		Spec: corev1.PodSpec{
			NodeName: n1.Name,
		},
		Status: corev1.PodStatus{
			PodIP: "10.14.7.12",
			Phase: corev1.PodRunning,
		},
	}
	thirtySecsAgo := metav1.NewTime(time.Now().UTC().Add(-30 * time.Second))

	// pod exited recently, should be resolved if no other pod shares the IP
	p2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "p2",
			Namespace:         "team1",
			CreationTimestamp: thirtySecsAgo,
		},
		Spec: corev1.PodSpec{
			NodeName: n1.Name,
		},
		Status: corev1.PodStatus{
			PodIP: "10.14.7.12",
			Phase: corev1.PodSucceeded,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: thirtySecsAgo,
					Reason:             "PodCompleted",
				},
			},
		},
	}

	// pod exited recently, should be resolved
	p3 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "p3",
			Namespace:         "team1",
			CreationTimestamp: thirtySecsAgo,
		},
		Spec: corev1.PodSpec{
			NodeName: n1.Name,
		},
		Status: corev1.PodStatus{
			PodIP: "10.14.7.13",
			Phase: corev1.PodSucceeded,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: thirtySecsAgo,
					Reason:             "PodCompleted",
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(n1, p1, p2, p3)

	informersFactory := informers.NewSharedInformerFactoryWithOptions(clientset, 30*time.Second)
	podsInformer := informersFactory.Core().V1().Pods().Informer()
	nodesInformer := informersFactory.Core().V1().Nodes().Informer()
	podsByNodeCache := NewPodsByNodeCache(podsInformer)
	podByIPCache := NewPodByIPCache(context.Background(), podsInformer, logrus.New())
	nodeByNameCache := NewNodeByNameCache(nodesInformer)
	nodeByIPCache := NewNodeByIPCache(nodesInformer)
	informersFactory.Start(wait.NeverStop)
	informersFactory.WaitForCacheSync(wait.NeverStop)

	t.Run("get node by ip", func(t *testing.T) {
		r := require.New(t)
		n, err := nodeByIPCache.Get(n1.Status.Addresses[0].Address)
		r.NoError(err)
		r.Equal(n1, n)
	})

	t.Run("get pod by ip: when there are two pods with the same IP, prefers newest running", func(t *testing.T) {
		r := require.New(t)
		p, err := podByIPCache.Get(p1.Status.PodIP)
		r.NoError(err)
		r.Equal(p1, p)
	})

	t.Run("get recently stopped pod by ip", func(t *testing.T) {
		r := require.New(t)
		p, err := podByIPCache.Get(p3.Status.PodIP)
		r.NoError(err)
		r.Equal(p3, p)
	})

	t.Run("nonexistent pod by IP", func(t *testing.T) {
		r := require.New(t)
		_, err := podByIPCache.Get("nonexistentip")
		r.ErrorIs(err, ErrNotFound)
	})

	t.Run("get running pods by node name", func(t *testing.T) {
		r := require.New(t)
		pods, err := podsByNodeCache.Get(n1.Name)
		r.NoError(err)
		r.Equal(p1, pods[0])
	})

	t.Run("get node by node name", func(t *testing.T) {
		r := require.New(t)
		node, err := nodeByNameCache.Get(n1.Name)
		r.NoError(err)
		r.Equal(n1, node)
	})

	t.Run("return no object for unknown pod ip", func(t *testing.T) {
		r := require.New(t)
		_, err := podByIPCache.Get("1.1.1.1")
		r.EqualError(err, ErrNotFound.Error())
	})
}
