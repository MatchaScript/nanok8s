package pki

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// ensureServiceAccountKey generates or reuses the RSA key pair used by
// kube-controller-manager to sign ServiceAccount tokens and by kube-apiserver
// to verify them. Unlike every other file in the PKI dir, this is a bare
// key pair, not a certificate.
func ensureServiceAccountKey(pkiDir string) (created bool, err error) {
	keyPath := filepath.Join(pkiDir, "sa.key")
	pubPath := filepath.Join(pkiDir, "sa.pub")

	_, errKey := os.Stat(keyPath)
	_, errPub := os.Stat(pubPath)
	if errKey == nil && errPub == nil {
		if _, err := loadKey(keyPath); err != nil {
			return false, fmt.Errorf("reuse sa.key: %w", err)
		}
		return false, nil
	}

	key, err := generateKey()
	if err != nil {
		return false, err
	}
	if err := writeKey(keyPath, key); err != nil {
		return false, err
	}
	if err := writePublicKey(pubPath, &key.PublicKey); err != nil {
		return false, err
	}
	return true, nil
}

func writePublicKey(path string, pub *rsa.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return err
	}
	buf := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	return os.WriteFile(path, buf, certFileMode)
}
