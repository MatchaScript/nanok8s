package kubeadm

import (
	"fmt"
	"os"

	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/controlplane"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/etcd"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/kubelet"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
)

// Ensure runs the full bootstrap pipeline in dependency order:
//
//	PKI -> kubeconfigs -> etcd static pod -> control plane static pods -> kubelet config
//
// Each kubeadm phase is internally idempotent: existing, valid files are
// preserved; missing files are created. Ensure is safe to re-run on every
// boot and after `nanok8s apply`.
func Ensure(cfg *v1alpha1.NanoK8sConfig, layout Layout, nodeName string) error {
	kc, err := BuildInitConfiguration(cfg, layout, nodeName)
	if err != nil {
		return err
	}

	// kubeadm's phases assume the destination directories exist.
	for _, dir := range []string{layout.PKIDir, layout.KubeconfigDir, layout.ManifestsDir, layout.KubeletDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	if err := certs.CreatePKIAssets(kc); err != nil {
		return fmt.Errorf("create PKI assets: %w", err)
	}

	// CreateJoinControlPlaneKubeConfigFiles covers admin/controller-manager/scheduler.
	// kubelet.conf is produced separately because the join flow delegates that
	// file to a bootstrap-token exchange which nanok8s does not use.
	for _, name := range []string{
		kubeadmconstants.AdminKubeConfigFileName,
		kubeadmconstants.ControllerManagerKubeConfigFileName,
		kubeadmconstants.SchedulerKubeConfigFileName,
		kubeadmconstants.KubeletKubeConfigFileName,
	} {
		if err := kubeconfig.CreateKubeConfigFile(name, layout.KubeconfigDir, kc); err != nil {
			return fmt.Errorf("create kubeconfig %s: %w", name, err)
		}
	}

	// patchesDir is unused in v0 (no user-supplied kustomize-style patches).
	const patchesDir = ""
	const isDryRun = false

	if err := etcd.CreateLocalEtcdStaticPodManifestFile(
		layout.ManifestsDir, patchesDir, nodeName, &kc.ClusterConfiguration, &kc.LocalAPIEndpoint, isDryRun,
	); err != nil {
		return fmt.Errorf("create etcd manifest: %w", err)
	}

	if err := controlplane.CreateInitStaticPodManifestFiles(
		layout.ManifestsDir, patchesDir, kc, isDryRun,
	); err != nil {
		return fmt.Errorf("create control plane manifests: %w", err)
	}

	// kubelet phase: ordering mirrors kubeadm's init kubelet-start.
	// WriteConfigToDisk reads the instance file as a patch when the
	// NodeLocalCRISocket feature gate is on (GA+locked as of k8s v1.36),
	// so the instance file must be written first.
	if err := kubelet.WriteKubeletDynamicEnvFile(&kc.ClusterConfiguration, &kc.NodeRegistration, false, layout.KubeletDir); err != nil {
		return fmt.Errorf("write kubelet env file: %w", err)
	}
	instance := &kubeletconfigv1beta1.KubeletConfiguration{
		ContainerRuntimeEndpoint: kc.NodeRegistration.CRISocket,
	}
	if err := kubelet.WriteInstanceConfigToDisk(instance, layout.KubeletDir); err != nil {
		return fmt.Errorf("write kubelet instance config: %w", err)
	}
	if err := kubelet.WriteConfigToDisk(&kc.ClusterConfiguration, layout.KubeletDir, patchesDir, os.Stderr); err != nil {
		return fmt.Errorf("write kubelet config: %w", err)
	}

	return nil
}
