// Package kubeclient builds a Kubernetes client against a nanok8s-managed
// control plane using admin.conf, and provides readiness gates callers
// use before applying server-side resources.
package kubeclient

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// LoadAdmin builds a typed clientset from the kubeconfig at path
// (usually /etc/kubernetes/admin.conf).
func LoadAdmin(path string) (kubernetes.Interface, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("build rest config from %s: %w", path, err)
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	return clientset, nil
}

// WaitForAPIServer polls the apiserver's /readyz endpoint until it succeeds
// or ctx is cancelled. Invoked from lifecycle.Boot after kubelet is started
// and must tolerate the apiserver static pod not yet being up. The caller
// controls the deadline via ctx (e.g. context.WithTimeout).
func WaitForAPIServer(ctx context.Context, client kubernetes.Interface) error {
	var lastErr error
	for {
		_, err := client.Discovery().RESTClient().Get().AbsPath("/readyz").DoRaw(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("apiserver not ready: %w (last probe: %v)", ctx.Err(), lastErr)
			}
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// WaitForControlPlane polls in parallel for four conditions to all hold:
//
//   - Node <nodeName> reports Ready=True
//   - Static pod kube-apiserver-<nodeName> reports Ready=True
//   - Static pod kube-controller-manager-<nodeName> reports Ready=True
//   - Static pod kube-scheduler-<nodeName> reports Ready=True
//
// Mirrors kinder's waitNewControlPlaneNodeReady
// (reference/kubeadm/kinder/pkg/cluster/manager/actions/waiter.go).
// Returns nil when all four pass; returns ctx.Err() (wrapped) on timeout.
// Requires an authenticated client; call EnsureAdminRBAC first.
func WaitForControlPlane(ctx context.Context, client kubernetes.Interface, nodeName string) error {
	checks := []readyCheck{
		{name: fmt.Sprintf("node/%s", nodeName), fn: func(ctx context.Context) bool { return nodeReady(ctx, client, nodeName) }},
		{name: fmt.Sprintf("pod/kube-apiserver-%s", nodeName), fn: func(ctx context.Context) bool {
			return staticPodReady(ctx, client, "kube-apiserver-"+nodeName)
		}},
		{name: fmt.Sprintf("pod/kube-controller-manager-%s", nodeName), fn: func(ctx context.Context) bool {
			return staticPodReady(ctx, client, "kube-controller-manager-"+nodeName)
		}},
		{name: fmt.Sprintf("pod/kube-scheduler-%s", nodeName), fn: func(ctx context.Context) bool {
			return staticPodReady(ctx, client, "kube-scheduler-"+nodeName)
		}},
	}
	return waitAllReady(ctx, checks)
}

type readyCheck struct {
	name string
	fn   func(context.Context) bool
}

func waitAllReady(ctx context.Context, checks []readyCheck) error {
	passed := make(chan string, len(checks))
	for _, c := range checks {
		c := c
		go func() {
			for {
				if c.fn(ctx) {
					passed <- c.name
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			}
		}()
	}
	remaining := len(checks)
	for remaining > 0 {
		select {
		case <-passed:
			remaining--
		case <-ctx.Done():
			return fmt.Errorf("control plane not ready: %w (%d/%d checks still pending)",
				ctx.Err(), remaining, len(checks))
		}
	}
	return nil
}

func nodeReady(ctx context.Context, client kubernetes.Interface, nodeName string) bool {
	node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return false
	}
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func staticPodReady(ctx context.Context, client kubernetes.Interface, podName string) bool {
	pod, err := client.CoreV1().Pods(metav1.NamespaceSystem).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return false
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
