package kubeadm

import (
	"os"
	"path/filepath"
	"testing"

	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"
	"k8s.io/kubernetes/cmd/kubeadm/app/componentconfigs"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
)

func testConfig() *v1alpha1.NanoK8sConfig {
	c := &v1alpha1.NanoK8sConfig{
		Metadata: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.NanoK8sConfigSpec{
			ControlPlane: v1alpha1.ControlPlaneSpec{AdvertiseAddress: "192.168.10.10"},
			Certificates: v1alpha1.CertificatesSpec{
				SelfSigned: true,
				ExtraSANs:  []string{"nanok8s.local", "10.0.0.5"},
			},
		},
	}
	v1alpha1.SetDefaults(c)
	return c
}

func testLayout(t *testing.T) Layout {
	t.Helper()
	root := t.TempDir()
	return Layout{
		PKIDir:        filepath.Join(root, "pki"),
		KubeconfigDir: filepath.Join(root, "kubernetes"),
		ManifestsDir:  filepath.Join(root, "kubernetes", "manifests"),
		KubeletDir:    filepath.Join(root, "var", "lib", "kubelet"),
	}
}

func TestBuildInitConfigurationPopulatesCoreFields(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)

	kc, err := BuildInitConfiguration(cfg, layout, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	if kc.NodeRegistration.Name != "node-1" {
		t.Errorf("NodeRegistration.Name=%q, want node-1", kc.NodeRegistration.Name)
	}
	if kc.LocalAPIEndpoint.AdvertiseAddress != "192.168.10.10" {
		t.Errorf("AdvertiseAddress=%q", kc.LocalAPIEndpoint.AdvertiseAddress)
	}
	if kc.LocalAPIEndpoint.BindPort != 6443 {
		t.Errorf("BindPort=%d, want 6443", kc.LocalAPIEndpoint.BindPort)
	}
	if kc.Networking.ServiceSubnet != "10.96.0.0/12" {
		t.Errorf("ServiceSubnet=%q", kc.Networking.ServiceSubnet)
	}
	if kc.CertificatesDir != layout.PKIDir {
		t.Errorf("CertificatesDir=%q, want %q", kc.CertificatesDir, layout.PKIDir)
	}
	if kc.Etcd.Local == nil || len(kc.Etcd.Local.ServerCertSANs) != 2 {
		t.Errorf("Etcd.Local.ServerCertSANs=%v", kc.Etcd.Local)
	}
	if len(kc.APIServer.CertSANs) != 2 {
		t.Errorf("APIServer.CertSANs=%v, want 2 entries", kc.APIServer.CertSANs)
	}

	kubeletCfg := kc.ComponentConfigs[componentconfigs.KubeletGroup].Get().(*kubeletconfigv1beta1.KubeletConfiguration)
	if kubeletCfg.CgroupDriver != "systemd" {
		t.Errorf("KubeletConfiguration.CgroupDriver=%q, want systemd", kubeletCfg.CgroupDriver)
	}
	if len(kubeletCfg.ClusterDNS) != 1 || kubeletCfg.ClusterDNS[0] != "10.96.0.10" {
		t.Errorf("KubeletConfiguration.ClusterDNS=%v, want [10.96.0.10]", kubeletCfg.ClusterDNS)
	}

	if kc.CACertificateValidityPeriod == nil || kc.CACertificateValidityPeriod.Duration.Hours() != 3650*24 {
		t.Errorf("CACertificateValidityPeriod=%v, want 3650d", kc.CACertificateValidityPeriod)
	}
	if kc.CertificateValidityPeriod == nil || kc.CertificateValidityPeriod.Duration.Hours() != 3650*24 {
		t.Errorf("CertificateValidityPeriod=%v, want 3650d", kc.CertificateValidityPeriod)
	}
}

func TestEnsureProducesExpectedArtifacts(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)
	if err := Ensure(cfg, layout, "node-1"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	checks := []string{
		filepath.Join(layout.PKIDir, "ca.crt"),
		filepath.Join(layout.PKIDir, "ca.key"),
		filepath.Join(layout.PKIDir, "apiserver.crt"),
		filepath.Join(layout.PKIDir, "front-proxy-ca.crt"),
		filepath.Join(layout.PKIDir, "sa.key"),
		filepath.Join(layout.PKIDir, "etcd", "ca.crt"),
		filepath.Join(layout.PKIDir, "etcd", "server.crt"),
		filepath.Join(layout.KubeconfigDir, "admin.conf"),
		filepath.Join(layout.KubeconfigDir, "controller-manager.conf"),
		filepath.Join(layout.KubeconfigDir, "scheduler.conf"),
		filepath.Join(layout.KubeconfigDir, "kubelet.conf"),
		filepath.Join(layout.ManifestsDir, "etcd.yaml"),
		filepath.Join(layout.ManifestsDir, "kube-apiserver.yaml"),
		filepath.Join(layout.ManifestsDir, "kube-controller-manager.yaml"),
		filepath.Join(layout.ManifestsDir, "kube-scheduler.yaml"),
		filepath.Join(layout.KubeletDir, "config.yaml"),
	}
	for _, p := range checks {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing artifact: %s (%v)", p, err)
		}
	}
}

func TestEnsureIsIdempotent(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)
	if err := Ensure(cfg, layout, "node-1"); err != nil {
		t.Fatal(err)
	}
	// Capture PKI file contents: kubeadm's phases preserve existing valid
	// certs, so a second run must not rotate them.
	before, err := os.ReadFile(filepath.Join(layout.PKIDir, "apiserver.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Ensure(cfg, layout, "node-1"); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(layout.PKIDir, "apiserver.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("apiserver.crt was rewritten by second Ensure; phases should reuse existing valid certs")
	}
}
