package pki

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
)

func testConfig() *v1alpha1.NanoK8sConfig {
	c := &v1alpha1.NanoK8sConfig{
		Metadata: v1alpha1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.NanoK8sConfigSpec{
			ControlPlane: v1alpha1.ControlPlaneSpec{
				AdvertiseAddress: "192.168.10.10",
			},
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
		PKIDir:     filepath.Join(root, "pki"),
		EtcdPKIDir: filepath.Join(root, "pki", "etcd"),
	}
}

func TestEnsureFromScratchThenReuse(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)

	first, err := Ensure(cfg, layout)
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	for _, item := range first.Items {
		if item.Action != ActionCreated {
			t.Errorf("first run %s: want created, got %s", item.ID, item.Action)
		}
	}

	second, err := Ensure(cfg, layout)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	for _, item := range second.Items {
		if item.Action != ActionReused {
			t.Errorf("second run %s: want reused, got %s (%s)", item.ID, item.Action, item.Reason)
		}
	}
}

func TestFilesHaveExpectedPermissions(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)
	if _, err := Ensure(cfg, layout); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path string
		mode os.FileMode
	}{
		{filepath.Join(layout.PKIDir, "ca.crt"), 0o644},
		{filepath.Join(layout.PKIDir, "ca.key"), 0o600},
		{filepath.Join(layout.PKIDir, "sa.key"), 0o600},
		{filepath.Join(layout.PKIDir, "sa.pub"), 0o644},
		{filepath.Join(layout.EtcdPKIDir, "server.key"), 0o600},
	}
	for _, tc := range tests {
		info, err := os.Stat(tc.path)
		if err != nil {
			t.Errorf("%s: %v", tc.path, err)
			continue
		}
		if got := info.Mode().Perm(); got != tc.mode {
			t.Errorf("%s: perm %o, want %o", tc.path, got, tc.mode)
		}
	}
}

func TestApiserverCertSANs(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)
	if _, err := Ensure(cfg, layout); err != nil {
		t.Fatal(err)
	}
	cert, err := loadCert(filepath.Join(layout.PKIDir, "apiserver.crt"))
	if err != nil {
		t.Fatal(err)
	}

	wantDNS := map[string]bool{
		"kubernetes":                           true,
		"kubernetes.default":                   true,
		"kubernetes.default.svc":               true,
		"kubernetes.default.svc.cluster.local": true,
		"nanok8s.local":                        true,
	}
	for _, d := range cert.DNSNames {
		delete(wantDNS, d)
	}
	if len(wantDNS) != 0 {
		t.Errorf("apiserver cert missing DNS SANs: %v (have %v)", wantDNS, cert.DNSNames)
	}

	wantIPs := map[string]bool{
		"10.96.0.1":     true, // first IP of default service subnet
		"192.168.10.10": true, // advertiseAddress
		"10.0.0.5":      true, // from extraSANs
	}
	for _, ip := range cert.IPAddresses {
		delete(wantIPs, ip.String())
	}
	if len(wantIPs) != 0 {
		t.Errorf("apiserver cert missing IP SANs: %v (have %v)", wantIPs, cert.IPAddresses)
	}
}

func TestLeavesAreSignedByExpectedCA(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)
	if _, err := Ensure(cfg, layout); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		leaf, ca string
	}{
		{"apiserver", "ca"},
		{"apiserver-kubelet-client", "ca"},
		{"front-proxy-client", "front-proxy-ca"},
		{"apiserver-etcd-client", filepath.Join("etcd", "ca")},
	}
	for _, tc := range tests {
		leafCert, err := loadCert(filepath.Join(layout.PKIDir, tc.leaf+".crt"))
		if err != nil {
			t.Fatalf("load %s: %v", tc.leaf, err)
		}
		caCert, err := loadCert(filepath.Join(filepath.Dir(layout.PKIDir), filepath.Base(layout.PKIDir), tc.ca+".crt"))
		if err != nil {
			t.Fatalf("load CA %s: %v", tc.ca, err)
		}
		pool := x509.NewCertPool()
		pool.AddCert(caCert)
		if _, err := leafCert.Verify(x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		}); err != nil {
			t.Errorf("%s not verified against %s: %v", tc.leaf, tc.ca, err)
		}
	}
}

func TestStaleLeavesAreReissuedAfterCARegeneration(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)
	if _, err := Ensure(cfg, layout); err != nil {
		t.Fatal(err)
	}

	// Remove the main CA but leave leaves in place. Next Ensure should
	// regenerate the CA and re-issue everything that depended on it.
	if err := os.Remove(filepath.Join(layout.PKIDir, "ca.crt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(layout.PKIDir, "ca.key")); err != nil {
		t.Fatal(err)
	}

	report, err := Ensure(cfg, layout)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ReportItem{}
	for _, item := range report.Items {
		byID[item.ID] = item
	}
	for _, id := range []string{"ca", "apiserver", "apiserver-kubelet-client"} {
		if byID[id].Action != ActionCreated {
			t.Errorf("%s: want reissued, got %s (%s)", id, byID[id].Action, byID[id].Reason)
		}
	}
	// front-proxy-ca and etcd-ca should have been reused.
	for _, id := range []string{"front-proxy-ca", "etcd-ca"} {
		if byID[id].Action != ActionReused {
			t.Errorf("%s: want reused, got %s (%s)", id, byID[id].Action, byID[id].Reason)
		}
	}
}

