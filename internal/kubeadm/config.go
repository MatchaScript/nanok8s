// Package kubeadm adapts nanok8s configuration to the Go API exposed by
// k8s.io/kubernetes/cmd/kubeadm/app. nanok8s reuses kubeadm phases
// (certs, kubeconfig, controlplane, etcd, kubelet) as a library rather
// than shelling out to the kubeadm CLI. The orchestration mirrors the
// approach used by vcluster (pkg/certs/ensure.go).
package kubeadm

import (
	"fmt"

	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmconfig "k8s.io/kubernetes/cmd/kubeadm/app/util/config"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
	"github.com/MatchaScript/nanok8s/internal/paths"
)

// Layout selects the on-disk directories used by Ensure. Production callers
// pass the paths defaults; tests use t.TempDir-derived values.
type Layout struct {
	PKIDir        string // where certs.CreatePKIAssets writes
	KubeconfigDir string // where kubeconfig.CreateJoinControlPlaneKubeConfigFiles writes
	ManifestsDir  string // where static pod manifests land
	KubeletDir    string // where kubelet config.yaml lands
}

// DefaultLayout returns the production layout rooted at /etc/kubernetes
// and /var/lib/kubelet.
func DefaultLayout() Layout {
	return Layout{
		PKIDir:        paths.PKIDir,
		KubeconfigDir: paths.KubeconfigDir,
		ManifestsDir:  paths.ManifestsDir,
		KubeletDir:    paths.KubeletDir,
	}
}

// BuildInitConfiguration translates a NanoK8sConfig into the kubeadm
// InitConfiguration consumed by every phase below. It is exported so that
// future operator-style callers (k0smotron-equivalent) can inject extra
// overrides before calling phases themselves.
func BuildInitConfiguration(cfg *v1alpha1.NanoK8sConfig, layout Layout, nodeName string) (*kubeadmapi.InitConfiguration, error) {
	kc, err := kubeadmconfig.DefaultedStaticInitConfiguration()
	if err != nil {
		return nil, fmt.Errorf("kubeadm default init config: %w", err)
	}

	kc.ClusterName = "kubernetes"
	kc.KubernetesVersion = cfg.Spec.KubernetesVersion
	kc.CertificatesDir = layout.PKIDir

	kc.NodeRegistration.Name = nodeName
	kc.NodeRegistration.CRISocket = cfg.Spec.Runtime.CRISocket

	kc.LocalAPIEndpoint.AdvertiseAddress = cfg.Spec.ControlPlane.AdvertiseAddress
	kc.LocalAPIEndpoint.BindPort = cfg.Spec.ControlPlane.BindPort

	kc.Networking.ServiceSubnet = cfg.Spec.ControlPlane.ServiceSubnet
	kc.Networking.PodSubnet = cfg.Spec.ControlPlane.PodSubnet
	kc.Networking.DNSDomain = "cluster.local"

	kc.Etcd = kubeadmapi.Etcd{
		Local: &kubeadmapi.LocalEtcd{
			ServerCertSANs: extraSANs(cfg),
			PeerCertSANs:   extraSANs(cfg),
		},
	}

	kc.APIServer.CertSANs = extraSANs(cfg)

	return kc, nil
}

// extraSANs returns the user-declared SANs verbatim. kubeadm's phases/certs
// classifies each entry as DNS or IP internally, so we do not pre-split.
func extraSANs(cfg *v1alpha1.NanoK8sConfig) []string {
	out := make([]string, len(cfg.Spec.Certificates.ExtraSANs))
	copy(out, cfg.Spec.Certificates.ExtraSANs)
	return out
}
