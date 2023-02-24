package kube

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWatcher(t *testing.T) {
	r := require.New(t)

	n1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "n1",
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
	p2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p2",
			Namespace: "team1",
		},
		Spec: corev1.PodSpec{
			NodeName: n1.Name,
		},
		Status: corev1.PodStatus{
			PodIP: "10.14.7.12",
			Phase: corev1.PodSucceeded,
		},
	}
	clientset := fake.NewSimpleClientset(n1, p1, p2)
	w := NewWatcher(clientset)

	t.Run("get node by ip", func(t *testing.T) {
		n, err := w.GetNodeByIP(n1.Status.Addresses[0].Address)
		r.NoError(err)
		r.Equal(n1, n)
	})

	t.Run("get pod by ip", func(t *testing.T) {
		p, err := w.GetPodByIP(p1.Status.PodIP)
		r.NoError(err)
		r.Equal(p1, p)
	})

	t.Run("get running pods by node name", func(t *testing.T) {
		pods, err := w.GetPodsByNode(n1.Name)
		r.NoError(err)
		r.Equal(p1, pods[0])
	})

	t.Run("get node by node name", func(t *testing.T) {
		node, err := w.GetNodeByName(n1.Name)
		r.NoError(err)
		r.Equal(n1, node)
	})

	t.Run("return no object for unknown pod ip", func(t *testing.T) {
		_, err := w.GetPodByIP("1.1.1.1")
		r.EqualError(err, ErrNotFound.Error())
	})
}
