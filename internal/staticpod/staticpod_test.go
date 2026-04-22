package staticpod

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
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

func testLayout(t *testing.T) Layout {
	t.Helper()
	return Layout{ManifestsDir: t.TempDir()}
}

func TestEnsureCreatesAllFour(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)

	report, err := Ensure(cfg, layout, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := map[string]bool{
		"etcd": true, "kube-apiserver": true,
		"kube-controller-manager": true, "kube-scheduler": true,
	}
	for _, item := range report.Items {
		if item.Action != ActionCreated {
			t.Errorf("%s: want created, got %s", item.ID, item.Action)
		}
		delete(wantIDs, item.ID)
		if _, err := os.Stat(item.File); err != nil {
			t.Errorf("%s: file missing: %v", item.ID, err)
		}
	}
	if len(wantIDs) != 0 {
		t.Errorf("missing manifests: %v", wantIDs)
	}
}

func TestSecondEnsureReusesUnchangedManifests(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)

	if _, err := Ensure(cfg, layout, "node-1"); err != nil {
		t.Fatal(err)
	}
	report, err := Ensure(cfg, layout, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Items {
		if item.Action != ActionReused {
			t.Errorf("%s: want reused, got %s", item.ID, item.Action)
		}
	}
}

func TestConfigChangeTriggersUpdate(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)

	if _, err := Ensure(cfg, layout, "node-1"); err != nil {
		t.Fatal(err)
	}
	cfg.Spec.ControlPlane.AdvertiseAddress = "10.0.0.99"
	report, err := Ensure(cfg, layout, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	// etcd + kube-apiserver reference advertiseAddress; cm + scheduler do not.
	byID := map[string]Action{}
	for _, item := range report.Items {
		byID[item.ID] = item.Action
	}
	if byID["etcd"] != ActionUpdated {
		t.Errorf("etcd: want updated, got %s", byID["etcd"])
	}
	if byID["kube-apiserver"] != ActionUpdated {
		t.Errorf("kube-apiserver: want updated, got %s", byID["kube-apiserver"])
	}
	if byID["kube-controller-manager"] != ActionReused {
		t.Errorf("kube-controller-manager: want reused, got %s", byID["kube-controller-manager"])
	}
	if byID["kube-scheduler"] != ActionReused {
		t.Errorf("kube-scheduler: want reused, got %s", byID["kube-scheduler"])
	}
}

func TestManifestsAreValidPods(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)
	if _, err := Ensure(cfg, layout, "node-1"); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"etcd.yaml", "kube-apiserver.yaml", "kube-controller-manager.yaml", "kube-scheduler.yaml"} {
		data, err := os.ReadFile(filepath.Join(layout.ManifestsDir, name))
		if err != nil {
			t.Fatal(err)
		}
		var parsed pod
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			t.Errorf("%s: unmarshal: %v", name, err)
			continue
		}
		if parsed.APIVersion != "v1" || parsed.Kind != "Pod" {
			t.Errorf("%s: apiVersion=%q kind=%q", name, parsed.APIVersion, parsed.Kind)
		}
		if parsed.Metadata.Namespace != "kube-system" {
			t.Errorf("%s: namespace=%q", name, parsed.Metadata.Namespace)
		}
		if len(parsed.Spec.Containers) != 1 {
			t.Errorf("%s: want 1 container, got %d", name, len(parsed.Spec.Containers))
		}
		if parsed.Spec.Containers[0].LivenessProbe == nil {
			t.Errorf("%s: missing livenessProbe", name)
		}
	}
}

func TestApiserverFlagsContainExpectedPaths(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)
	if _, err := Ensure(cfg, layout, "node-1"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(layout.ManifestsDir, "kube-apiserver.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	wants := []string{
		"--advertise-address=192.168.10.10",
		"--client-ca-file=/etc/kubernetes/pki/ca.crt",
		"--etcd-servers=https://192.168.10.10:2379",
		"--service-account-signing-key-file=/etc/kubernetes/pki/sa.key",
		"--service-cluster-ip-range=10.96.0.0/12",
		"--tls-cert-file=/etc/kubernetes/pki/apiserver.crt",
	}
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("kube-apiserver.yaml missing %q", w)
		}
	}
}

func TestEtcdFlagsUseNodeName(t *testing.T) {
	cfg := testConfig()
	layout := testLayout(t)
	if _, err := Ensure(cfg, layout, "edge-42"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(layout.ManifestsDir, "etcd.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "--name=edge-42") {
		t.Error("etcd.yaml missing --name=edge-42")
	}
	if !strings.Contains(s, "name: etcd-edge-42") {
		t.Error("etcd.yaml missing pod name etcd-edge-42")
	}
}
