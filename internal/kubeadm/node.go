package kubeadm

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/markcontrolplane"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
)

// EnsureAdminRBAC ensures the "kubeadm:cluster-admins" ClusterRoleBinding
// exists so that admin.conf (which is bound to that Group, not to the
// built-in system:masters) can authenticate. On a fresh cluster the
// admin.conf client is denied by RBAC and the call transparently falls
// back to super-admin.conf (system:masters-bound) to create the CRB.
// On subsequent boots the CRB already exists and this is a no-op returning
// the admin.conf-based client.
//
// Mirrors what kubeadm init does internally
// (k8s.io/kubernetes/cmd/kubeadm/app/cmd/init.go invokes this as part of
// bootstrapping its own client).
func EnsureAdminRBAC(layout Layout) (kubernetes.Interface, error) {
	client, err := kubeconfig.EnsureAdminClusterRoleBinding(layout.KubeconfigDir, nil)
	if err != nil {
		return nil, fmt.Errorf("ensure admin cluster role binding: %w", err)
	}
	return client, nil
}

// MarkControlPlane applies kubeadm's mark-control-plane phase to the live
// node: labels it with node-role.kubernetes.io/control-plane and applies
// the taints from cfg.Spec.NodeRegistration.Taints. This is what
// `kubeadm init` does after the apiserver becomes reachable; nanok8s
// calls it from lifecycle.Boot once /readyz succeeds.
func MarkControlPlane(client kubernetes.Interface, cfg *v1alpha1.NanoK8sConfig, nodeName string) error {
	if err := markcontrolplane.MarkControlPlane(client, nodeName, cfg.Spec.NodeRegistration.Taints); err != nil {
		return fmt.Errorf("mark control-plane: %w", err)
	}
	return nil
}
