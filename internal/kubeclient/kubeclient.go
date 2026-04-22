// Package kubeclient builds a Kubernetes client against a nanok8s-managed
// control plane using admin.conf, and provides a readiness gate callers
// use before applying addons or server-side resources.
package kubeclient

import (
	"context"
	"fmt"
	"time"

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
// or ctx is cancelled. Intended for use by `nanok8s addons apply`, which is
// typically started by systemd right after kubelet and must tolerate the
// apiserver container not yet being up. The caller controls the deadline
// via ctx (e.g. context.WithTimeout).
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
