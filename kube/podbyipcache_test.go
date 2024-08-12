package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	applyconfigurationscorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPodsByIPCache(t *testing.T) {
	ctx := context.Background()

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

	// pod exited recently, should be resolved
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
			PodIP: "10.14.7.13",
			Phase: corev1.PodRunning,
		},
	}

	setup := func() (*PodByIPCache, *fake.Clientset) {
		clientset := fake.NewSimpleClientset(n1, p1, p2)
		informersFactory := informers.NewSharedInformerFactoryWithOptions(clientset, 30*time.Second)
		podsInformer := informersFactory.Core().V1().Pods().Informer()
		podByIPCache := NewPodByIPCache(podsInformer, logrus.New())
		informersFactory.Start(wait.NeverStop)
		informersFactory.WaitForCacheSync(wait.NeverStop)
		return podByIPCache, clientset
	}

	t.Run("get pod by ip: when there are two pods with the same IP, prefers newest running", func(t *testing.T) {
		r := require.New(t)
		podByIPCache, _ := setup()
		p, err := podByIPCache.Get(p1.Status.PodIP)
		r.NoError(err)
		r.Equal(p1, p)
	})

	t.Run("get recently stopped pod by ip and delete after use", func(t *testing.T) {
		r := require.New(t)
		podByIPCache, clientset := setup()
		p2.Status.Phase = corev1.PodSucceeded
		_, err := clientset.CoreV1().Pods(p2.Namespace).ApplyStatus(ctx, &applyconfigurationscorev1.PodApplyConfiguration{
			TypeMetaApplyConfiguration: v1.TypeMetaApplyConfiguration{},
			ObjectMetaApplyConfiguration: &v1.ObjectMetaApplyConfiguration{
				Name:      lo.ToPtr(p2.Name),
				Namespace: lo.ToPtr(p2.Namespace),
			},
			Status: &applyconfigurationscorev1.PodStatusApplyConfiguration{
				Phase: lo.ToPtr(corev1.PodSucceeded),
			},
		}, metav1.ApplyOptions{})
		r.NoError(err)

		r.Eventually(func() bool {
			p, err := podByIPCache.Get(p2.Status.PodIP)
			r.NoError(err)
			r.Equal(p2.Name, p.Name)
			return p.Status.Phase == corev1.PodSucceeded
		}, 1*time.Second, 1*time.Millisecond)

		go podByIPCache.runCleanupExpiredLoop(ctx)

		r.Eventually(func() bool {
			_, err := podByIPCache.Get(p2.Status.PodIP)
			return errors.Is(err, ErrNotFound)
		}, 1*time.Second, 1*time.Millisecond)

	})

	t.Run("nonexistent pod by IP", func(t *testing.T) {
		r := require.New(t)
		podByIPCache, _ := setup()
		_, err := podByIPCache.Get("nonexistentip")
		r.ErrorIs(err, ErrNotFound)
	})

	t.Run("return no object for unknown pod ip", func(t *testing.T) {
		r := require.New(t)
		podByIPCache, _ := setup()
		_, err := podByIPCache.Get("1.1.1.1")
		r.EqualError(err, ErrNotFound.Error())
	})

	t.Run("cleanup deleted pods", func(t *testing.T) {
		r := require.New(t)
		podByIPCache, clientset := setup()

		podByIPCache.cleanupInterval = 1 * time.Millisecond
		podByIPCache.podTimeout = 1 * time.Millisecond
		go podByIPCache.runCleanupExpiredLoop(ctx)

		r.NoError(clientset.CoreV1().Pods(p2.Namespace).Delete(ctx, p2.Name, metav1.DeleteOptions{}))

		r.Eventually(func() bool {
			_, err := podByIPCache.Get(p2.Status.PodIP)
			return errors.Is(err, ErrNotFound)
		}, 1*time.Second, 1*time.Millisecond)
	})
}
