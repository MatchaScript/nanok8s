// Package kubeconfig generates the four kubeconfig files that a nanok8s
// single-node control plane consumes. Layout matches kubeadm exactly so
// that kubectl --kubeconfig=/etc/kubernetes/admin.conf and crictl
// introspection tools behave identically to a kubeadm-built cluster.
//
// Each kubeconfig embeds a freshly-issued client certificate and its
// matching private key; no separate .crt/.key files are written beside
// the PKI tree. On reboot the existing kubeconfig is reused unless the
// embedded cert has expired, is issued by a stale CA, or the server URL
// no longer matches the config.
package kubeconfig

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
)

const (
	fileMode = 0o600
	dirMode  = 0o755
)

// Layout selects the on-disk directories used by Ensure. Production callers
// pass paths.PKIDir and paths.KubernetesDir; tests use t.TempDir.
type Layout struct {
	CADir         string // directory that holds ca.crt and ca.key
	KubeconfigDir string // directory to write *.conf files into
}

type Action string

const (
	ActionCreated Action = "created"
	ActionReused  Action = "reused"
)

type Report struct {
	Items []ReportItem
}

type ReportItem struct {
	ID     string
	Action Action
	Reason string
}

func (r *Report) add(id string, action Action, reason string) {
	r.Items = append(r.Items, ReportItem{ID: id, Action: action, Reason: reason})
}

// Ensure writes every kubeconfig, reusing existing files when possible.
// nodeName is embedded in kubelet.conf as system:node:<nodeName>; callers
// typically pass os.Hostname() at bootstrap time.
func Ensure(cfg *v1alpha1.NanoK8sConfig, layout Layout, nodeName string) (*Report, error) {
	caCert, err := loadCert(filepath.Join(layout.CADir, "ca.crt"))
	if err != nil {
		return nil, fmt.Errorf("load ca.crt: %w", err)
	}
	caKey, err := loadKey(filepath.Join(layout.CADir, "ca.key"))
	if err != nil {
		return nil, fmt.Errorf("load ca.key: %w", err)
	}

	server := serverURL(cfg)
	validity := time.Duration(cfg.Spec.Certificates.LeafValidityDays) * 24 * time.Hour
	caBlob, err := os.ReadFile(filepath.Join(layout.CADir, "ca.crt"))
	if err != nil {
		return nil, err
	}

	specs := []spec{
		{id: "admin", filename: "admin.conf", cn: "kubernetes-admin", orgs: []string{"system:masters"}, context: "kubernetes-admin@kubernetes"},
		{id: "controller-manager", filename: "controller-manager.conf", cn: "system:kube-controller-manager", context: "system:kube-controller-manager@kubernetes"},
		{id: "scheduler", filename: "scheduler.conf", cn: "system:kube-scheduler", context: "system:kube-scheduler@kubernetes"},
		{id: "kubelet", filename: "kubelet.conf", cn: "system:node:" + nodeName, orgs: []string{"system:nodes"}, context: "system:node:" + nodeName + "@kubernetes"},
	}

	report := &Report{}
	for _, s := range specs {
		reason, err := ensureOne(s, filepath.Join(layout.KubeconfigDir, s.filename), server, caBlob, caCert, caKey, validity)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.id, err)
		}
		if reason == "" {
			report.add(s.id, ActionReused, "")
		} else {
			report.add(s.id, ActionCreated, reason)
		}
	}
	return report, nil
}

type spec struct {
	id       string
	filename string
	cn       string
	orgs     []string
	context  string
}

// ensureOne writes one kubeconfig file. The returned reason is empty on reuse
// and explains the trigger for re-issuance otherwise.
func ensureOne(s spec, path, server string, caBlob []byte, caCert *x509.Certificate, caKey *rsa.PrivateKey, validity time.Duration) (string, error) {
	if reason := reuseableReason(path, server, caBlob, caCert); reason == "" {
		return "", nil
	} else if reason != "missing" {
		// fall through and regenerate, recording the detected reason
		return writeFreshKubeconfig(path, s, server, caBlob, caCert, caKey, validity, reason)
	}
	return writeFreshKubeconfig(path, s, server, caBlob, caCert, caKey, validity, "missing")
}

// reuseableReason returns "" when the existing kubeconfig at path can be
// kept as-is, or a short string describing why it must be regenerated.
// A "missing" return means the file does not exist.
func reuseableReason(path, wantServer string, wantCA []byte, caCert *x509.Certificate) string {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "unreadable"
	}
	var kc kubeconfigFile
	if err := yaml.Unmarshal(data, &kc); err != nil {
		return "unparseable"
	}
	if len(kc.Clusters) == 0 || len(kc.Users) == 0 {
		return "malformed"
	}
	if kc.Clusters[0].Cluster.Server != wantServer {
		return "server URL changed"
	}
	gotCA, _ := base64.StdEncoding.DecodeString(kc.Clusters[0].Cluster.CertificateAuthorityData)
	if string(gotCA) != string(wantCA) {
		return "CA data changed"
	}

	certPEM, _ := base64.StdEncoding.DecodeString(kc.Users[0].User.ClientCertificateData)
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "embedded cert unparseable"
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "embedded cert unparseable"
	}
	now := time.Now().UTC()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return "embedded cert expired"
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		return "embedded cert issued by stale CA"
	}
	return ""
}

func writeFreshKubeconfig(path string, s spec, server string, caBlob []byte, caCert *x509.Certificate, caKey *rsa.PrivateKey, validity time.Duration, reason string) (string, error) {
	cert, key, err := issueClientCert(s.cn, s.orgs, caCert, caKey, validity)
	if err != nil {
		return "", err
	}
	kc := kubeconfigFile{
		APIVersion:     "v1",
		Kind:           "Config",
		CurrentContext: s.context,
		Clusters: []namedCluster{{
			Name: "kubernetes",
			Cluster: clusterFields{
				Server:                   server,
				CertificateAuthorityData: base64.StdEncoding.EncodeToString(caBlob),
			},
		}},
		Contexts: []namedContext{{
			Name: s.context,
			Context: contextFields{
				Cluster: "kubernetes",
				User:    s.cn,
			},
		}},
		Users: []namedUser{{
			Name: s.cn,
			User: userFields{
				ClientCertificateData: base64.StdEncoding.EncodeToString(encodeCert(cert)),
				ClientKeyData:         base64.StdEncoding.EncodeToString(encodeKey(key)),
			},
		}},
	}
	data, err := yaml.Marshal(&kc)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, fileMode); err != nil {
		return "", err
	}
	return reason, nil
}

func serverURL(cfg *v1alpha1.NanoK8sConfig) string {
	host := cfg.Spec.ControlPlane.AdvertiseAddress
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		host = "[" + host + "]"
	}
	return "https://" + host + ":" + strconv.FormatInt(int64(cfg.Spec.ControlPlane.BindPort), 10)
}
