package kubeconfig

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
	"github.com/MatchaScript/nanok8s/internal/pki"
)

func testConfig() *v1alpha1.NanoK8sConfig {
	c := &v1alpha1.NanoK8sConfig{
		Metadata: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.NanoK8sConfigSpec{
			ControlPlane: v1alpha1.ControlPlaneSpec{AdvertiseAddress: "192.168.10.10"},
			Certificates: v1alpha1.CertificatesSpec{SelfSigned: true},
		},
	}
	v1alpha1.SetDefaults(c)
	return c
}

// setupPKI runs pki.Ensure into a temp directory so tests have a valid CA
// to sign kubeconfig client certs with.
func setupPKI(t *testing.T) (cfg *v1alpha1.NanoK8sConfig, layout Layout) {
	t.Helper()
	cfg = testConfig()
	root := t.TempDir()
	pkiLayout := pki.Layout{
		PKIDir:     filepath.Join(root, "pki"),
		EtcdPKIDir: filepath.Join(root, "pki", "etcd"),
	}
	if _, err := pki.Ensure(cfg, pkiLayout); err != nil {
		t.Fatalf("pki.Ensure: %v", err)
	}
	return cfg, Layout{
		CADir:         pkiLayout.PKIDir,
		KubeconfigDir: filepath.Join(root, "kubernetes"),
	}
}

func TestEnsureFromScratchThenReuse(t *testing.T) {
	cfg, layout := setupPKI(t)

	first, err := Ensure(cfg, layout, "node-1")
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	for _, item := range first.Items {
		if item.Action != ActionCreated {
			t.Errorf("first run %s: want created, got %s", item.ID, item.Action)
		}
	}

	second, err := Ensure(cfg, layout, "node-1")
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	for _, item := range second.Items {
		if item.Action != ActionReused {
			t.Errorf("second run %s: want reused, got %s (%s)", item.ID, item.Action, item.Reason)
		}
	}
}

func TestServerURLChangeForcesRegeneration(t *testing.T) {
	cfg, layout := setupPKI(t)
	if _, err := Ensure(cfg, layout, "node-1"); err != nil {
		t.Fatal(err)
	}
	cfg.Spec.ControlPlane.AdvertiseAddress = "10.0.0.7"
	report, err := Ensure(cfg, layout, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Items {
		if item.Action != ActionCreated || item.Reason != "server URL changed" {
			t.Errorf("%s: want created/server URL changed, got %s/%s", item.ID, item.Action, item.Reason)
		}
	}
}

func TestKubeletUsesNodeName(t *testing.T) {
	cfg, layout := setupPKI(t)
	if _, err := Ensure(cfg, layout, "edge-42"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(layout.KubeconfigDir, "kubelet.conf"))
	if err != nil {
		t.Fatal(err)
	}
	var kc kubeconfigFile
	if err := yaml.Unmarshal(data, &kc); err != nil {
		t.Fatal(err)
	}
	wantUser := "system:node:edge-42"
	if kc.Users[0].Name != wantUser {
		t.Errorf("kubelet.conf user name: got %q, want %q", kc.Users[0].Name, wantUser)
	}

	certPEM, _ := base64.StdEncoding.DecodeString(kc.Users[0].User.ClientCertificateData)
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != wantUser {
		t.Errorf("kubelet cert CN: got %q, want %q", cert.Subject.CommonName, wantUser)
	}
	foundOrg := false
	for _, o := range cert.Subject.Organization {
		if o == "system:nodes" {
			foundOrg = true
		}
	}
	if !foundOrg {
		t.Errorf("kubelet cert missing O=system:nodes, got %v", cert.Subject.Organization)
	}
}

func TestAllClientCertsVerifyAgainstCA(t *testing.T) {
	cfg, layout := setupPKI(t)
	if _, err := Ensure(cfg, layout, "node-1"); err != nil {
		t.Fatal(err)
	}
	caCert, err := loadCert(filepath.Join(layout.CADir, "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	for _, name := range []string{"admin.conf", "controller-manager.conf", "scheduler.conf", "kubelet.conf"} {
		data, err := os.ReadFile(filepath.Join(layout.KubeconfigDir, name))
		if err != nil {
			t.Fatal(err)
		}
		var kc kubeconfigFile
		if err := yaml.Unmarshal(data, &kc); err != nil {
			t.Fatal(err)
		}
		certPEM, _ := base64.StdEncoding.DecodeString(kc.Users[0].User.ClientCertificateData)
		block, _ := pem.Decode(certPEM)
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("%s: parse cert: %v", name, err)
		}
		if _, err := cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
			t.Errorf("%s: cert does not verify against ca: %v", name, err)
		}
	}
}

func TestPermissionsAre0600(t *testing.T) {
	cfg, layout := setupPKI(t)
	if _, err := Ensure(cfg, layout, "node-1"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"admin.conf", "kubelet.conf"} {
		info, err := os.Stat(filepath.Join(layout.KubeconfigDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s: perm %o, want 0600", name, info.Mode().Perm())
		}
	}
}
