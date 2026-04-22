// Package pki generates and maintains the self-signed PKI that a single-node
// nanok8s control plane needs. Layout mirrors kubeadm's /etc/kubernetes/pki/
// tree so that the generated files are interchangeable with kubeadm-built
// clusters (useful for debugging with kubectl/crictl and for future migrations).
package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	rsaBits       = 2048
	certFileMode  = 0o644
	keyFileMode   = 0o600
	dirMode       = 0o755
	serialBitSize = 128
)

func generateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, rsaBits)
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), serialBitSize)
	return rand.Int(rand.Reader, limit)
}

// newSelfSignedCA issues a self-signed CA with the given common name and validity.
func newSelfSignedCA(cn string, validity time.Duration) (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := generateKey()
	if err != nil {
		return nil, nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(validity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

type leafSpec struct {
	cn       string
	orgs     []string
	usages   []x509.ExtKeyUsage
	dnsNames []string
	ips      []net.IP
	validity time.Duration
}

// newLeafSignedBy issues a leaf certificate signed by caCert/caKey.
func newLeafSignedBy(caCert *x509.Certificate, caKey *rsa.PrivateKey, s leafSpec) (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := generateKey()
	if err != nil {
		return nil, nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: s.cn, Organization: s.orgs},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(s.validity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  s.usages,
		DNSNames:     s.dnsNames,
		IPAddresses:  s.ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func writeCert(path string, cert *x509.Certificate) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}
	buf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	return os.WriteFile(path, buf, certFileMode)
}

func writeKey(path string, key *rsa.PrivateKey) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	buf := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	return os.WriteFile(path, buf, keyFileMode)
}

func loadCert(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s: no PEM block found", path)
	}
	return x509.ParseCertificate(block.Bytes)
}

func loadKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s: no PEM block found", path)
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// loadPair loads a cert+key pair only if both files exist and parse.
// Missing files are reported as (nil, nil, nil) so callers can treat
// "not present" differently from "corrupted".
func loadPair(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, error) {
	_, errC := os.Stat(certPath)
	_, errK := os.Stat(keyPath)
	if os.IsNotExist(errC) && os.IsNotExist(errK) {
		return nil, nil, nil
	}
	cert, err := loadCert(certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load cert %s: %w", certPath, err)
	}
	key, err := loadKey(keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load key %s: %w", keyPath, err)
	}
	return cert, key, nil
}

func certValid(cert *x509.Certificate) bool {
	now := time.Now().UTC()
	return cert != nil && now.After(cert.NotBefore) && now.Before(cert.NotAfter)
}

// verifyIssuer confirms leaf was issued by ca. Catches the case where the CA
// was regenerated while stale leaves remained on disk.
func verifyIssuer(leaf, ca *x509.Certificate) error {
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	return err
}
