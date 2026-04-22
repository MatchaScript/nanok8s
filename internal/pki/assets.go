package pki

import (
	"crypto/x509"
	"fmt"
	"net"
	"path/filepath"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
)

// Layout selects the on-disk directories used for the PKI tree.
// Production callers pass paths.PKIDir / paths.EtcdPKIDir; tests use t.TempDir.
type Layout struct {
	PKIDir     string
	EtcdPKIDir string
}

// caAsset describes one self-signed CA. The files live at
// filepath.Join(dir, name+".crt") and filepath.Join(dir, name+".key").
type caAsset struct {
	id   string // stable identifier, used as map key
	dir  string
	name string
	cn   string
}

// leafAsset describes one leaf cert. sansFn produces SANs at reconcile
// time from the live config, so SAN changes propagate to the cert.
type leafAsset struct {
	id      string
	dir     string
	name    string
	caID    string
	cn      string
	orgs    []string
	usages  []x509.ExtKeyUsage
	sansFn  func(*v1alpha1.NanoK8sConfig) (dnsNames []string, ips []net.IP, err error)
}

func (a caAsset) certPath() string   { return filepath.Join(a.dir, a.name+".crt") }
func (a caAsset) keyPath() string    { return filepath.Join(a.dir, a.name+".key") }
func (a leafAsset) certPath() string { return filepath.Join(a.dir, a.name+".crt") }
func (a leafAsset) keyPath() string  { return filepath.Join(a.dir, a.name+".key") }

// cas returns the three CAs nanok8s maintains. Order is irrelevant because
// CAs do not depend on each other, but it determines display order in reports.
func cas(l Layout) []caAsset {
	return []caAsset{
		{id: "ca", dir: l.PKIDir, name: "ca", cn: "kubernetes"},
		{id: "front-proxy-ca", dir: l.PKIDir, name: "front-proxy-ca", cn: "front-proxy-ca"},
		{id: "etcd-ca", dir: l.EtcdPKIDir, name: "ca", cn: "etcd-ca"},
	}
}

// leaves returns every leaf cert. Each entry names its issuing CA via caID.
func leaves(l Layout) []leafAsset {
	return []leafAsset{
		{
			id:     "apiserver",
			dir:    l.PKIDir,
			name:   "apiserver",
			caID:   "ca",
			cn:     "kube-apiserver",
			usages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			sansFn: apiserverSANs,
		},
		{
			id:     "apiserver-kubelet-client",
			dir:    l.PKIDir,
			name:   "apiserver-kubelet-client",
			caID:   "ca",
			cn:     "kube-apiserver-kubelet-client",
			orgs:   []string{"system:masters"},
			usages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		},
		{
			id:     "front-proxy-client",
			dir:    l.PKIDir,
			name:   "front-proxy-client",
			caID:   "front-proxy-ca",
			cn:     "front-proxy-client",
			usages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		},
		{
			id:     "apiserver-etcd-client",
			dir:    l.PKIDir,
			name:   "apiserver-etcd-client",
			caID:   "etcd-ca",
			cn:     "kube-apiserver-etcd-client",
			orgs:   []string{"system:masters"},
			usages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		},
		{
			id:     "etcd-server",
			dir:    l.EtcdPKIDir,
			name:   "server",
			caID:   "etcd-ca",
			cn:     "kube-etcd",
			usages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			sansFn: etcdServingSANs,
		},
		{
			id:     "etcd-peer",
			dir:    l.EtcdPKIDir,
			name:   "peer",
			caID:   "etcd-ca",
			cn:     "kube-etcd-peer",
			usages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			sansFn: etcdServingSANs,
		},
		{
			id:     "etcd-healthcheck-client",
			dir:    l.EtcdPKIDir,
			name:   "healthcheck-client",
			caID:   "etcd-ca",
			cn:     "kube-etcd-healthcheck-client",
			orgs:   []string{"system:masters"},
			usages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		},
	}
}

// apiserverSANs returns SANs for the kube-apiserver serving cert.
// Matches kubeadm's GetAPIServerAltNames, minus the node name (nanok8s
// doesn't track one in v0).
func apiserverSANs(cfg *v1alpha1.NanoK8sConfig) ([]string, []net.IP, error) {
	advertise := net.ParseIP(cfg.Spec.ControlPlane.AdvertiseAddress)
	if advertise == nil {
		return nil, nil, fmt.Errorf("advertiseAddress %q is not a valid IP", cfg.Spec.ControlPlane.AdvertiseAddress)
	}
	svcIP, err := firstIP(cfg.Spec.ControlPlane.ServiceSubnet)
	if err != nil {
		return nil, nil, fmt.Errorf("serviceSubnet: %w", err)
	}
	dnsNames := []string{
		"kubernetes",
		"kubernetes.default",
		"kubernetes.default.svc",
		"kubernetes.default.svc.cluster.local",
	}
	ips := []net.IP{svcIP, advertise}
	dnsNames, ips = appendExtraSANs(dnsNames, ips, cfg.Spec.Certificates.ExtraSANs)
	return dnsNames, ips, nil
}

// etcdServingSANs returns SANs for the etcd serving and peer certs.
// Local etcd listens on advertiseAddress and loopback.
func etcdServingSANs(cfg *v1alpha1.NanoK8sConfig) ([]string, []net.IP, error) {
	advertise := net.ParseIP(cfg.Spec.ControlPlane.AdvertiseAddress)
	if advertise == nil {
		return nil, nil, fmt.Errorf("advertiseAddress %q is not a valid IP", cfg.Spec.ControlPlane.AdvertiseAddress)
	}
	return []string{"localhost"},
		[]net.IP{advertise, net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		nil
}

func appendExtraSANs(dns []string, ips []net.IP, extras []string) ([]string, []net.IP) {
	for _, s := range extras {
		if ip := net.ParseIP(s); ip != nil {
			ips = append(ips, ip)
		} else {
			dns = append(dns, s)
		}
	}
	return dns, ips
}

// firstIP returns the first usable address in a CIDR (e.g. 10.96.0.1 for
// 10.96.0.0/12), which is kube-apiserver's well-known ClusterIP.
func firstIP(cidr string) (net.IP, error) {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	ip := n.IP.To4()
	if ip == nil {
		ip = n.IP
	}
	out := make(net.IP, len(ip))
	copy(out, ip)
	out[len(out)-1]++
	return out, nil
}
