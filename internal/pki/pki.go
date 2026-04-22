package pki

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"time"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
)

// Action records what Ensure did with one asset.
type Action string

const (
	ActionCreated Action = "created"
	ActionReused  Action = "reused"
)

// Report enumerates the disposition of every asset touched by Ensure.
type Report struct {
	Items []ReportItem
}

type ReportItem struct {
	ID     string
	Action Action
	Reason string // populated when Action == created, explains why
}

func (r *Report) add(id string, action Action, reason string) {
	r.Items = append(r.Items, ReportItem{ID: id, Action: action, Reason: reason})
}

// Ensure generates any missing CAs, leaf certs, and the service-account key
// pair, reusing on-disk material when it is present, unexpired, and issued
// by the current CA. Safe to call repeatedly.
func Ensure(cfg *v1alpha1.NanoK8sConfig, layout Layout) (*Report, error) {
	caValidity := time.Duration(cfg.Spec.Certificates.CAValidityDays) * 24 * time.Hour
	leafValidity := time.Duration(cfg.Spec.Certificates.LeafValidityDays) * 24 * time.Hour

	caCerts := map[string]*x509.Certificate{}
	caKeys := map[string]*rsa.PrivateKey{}
	report := &Report{}

	for _, ca := range cas(layout) {
		cert, key, reason, err := ensureCA(ca, caValidity)
		if err != nil {
			return nil, fmt.Errorf("ca %s: %w", ca.id, err)
		}
		caCerts[ca.id] = cert
		caKeys[ca.id] = key
		if reason == "" {
			report.add(ca.id, ActionReused, "")
		} else {
			report.add(ca.id, ActionCreated, reason)
		}
	}

	for _, leaf := range leaves(layout) {
		caCert, ok := caCerts[leaf.caID]
		if !ok {
			return nil, fmt.Errorf("leaf %s references unknown CA %s", leaf.id, leaf.caID)
		}
		reason, err := ensureLeaf(leaf, cfg, caCert, caKeys[leaf.caID], leafValidity)
		if err != nil {
			return nil, fmt.Errorf("leaf %s: %w", leaf.id, err)
		}
		if reason == "" {
			report.add(leaf.id, ActionReused, "")
		} else {
			report.add(leaf.id, ActionCreated, reason)
		}
	}

	saCreated, err := ensureServiceAccountKey(layout.PKIDir)
	if err != nil {
		return nil, fmt.Errorf("service-account key: %w", err)
	}
	if saCreated {
		report.add("sa", ActionCreated, "missing")
	} else {
		report.add("sa", ActionReused, "")
	}

	return report, nil
}

// ensureCA returns the CA cert and key, creating them if needed.
// An empty reason indicates reuse; a non-empty reason says why a new CA
// was issued.
func ensureCA(ca caAsset, validity time.Duration) (*x509.Certificate, *rsa.PrivateKey, string, error) {
	cert, key, err := loadPair(ca.certPath(), ca.keyPath())
	if err != nil {
		return nil, nil, "", err
	}
	if cert != nil && key != nil && certValid(cert) {
		return cert, key, "", nil
	}

	reason := "missing"
	if cert != nil && !certValid(cert) {
		reason = "expired"
	}

	cert, key, err = newSelfSignedCA(ca.cn, validity)
	if err != nil {
		return nil, nil, "", err
	}
	if err := writeCert(ca.certPath(), cert); err != nil {
		return nil, nil, "", err
	}
	if err := writeKey(ca.keyPath(), key); err != nil {
		return nil, nil, "", err
	}
	return cert, key, reason, nil
}

func ensureLeaf(leaf leafAsset, cfg *v1alpha1.NanoK8sConfig, caCert *x509.Certificate, caKey *rsa.PrivateKey, validity time.Duration) (string, error) {
	existingCert, _, err := loadPair(leaf.certPath(), leaf.keyPath())
	if err != nil {
		return "", err
	}
	if existingCert != nil && certValid(existingCert) && verifyIssuer(existingCert, caCert) == nil {
		return "", nil
	}

	reason := "missing"
	if existingCert != nil {
		switch {
		case !certValid(existingCert):
			reason = "expired"
		case verifyIssuer(existingCert, caCert) != nil:
			reason = "issued by stale CA"
		}
	}

	spec := leafSpec{
		cn:       leaf.cn,
		orgs:     leaf.orgs,
		usages:   leaf.usages,
		validity: validity,
	}
	if leaf.sansFn != nil {
		dns, ips, err := leaf.sansFn(cfg)
		if err != nil {
			return "", err
		}
		spec.dnsNames = dns
		spec.ips = ips
	}

	cert, key, err := newLeafSignedBy(caCert, caKey, spec)
	if err != nil {
		return "", err
	}
	if err := writeCert(leaf.certPath(), cert); err != nil {
		return "", err
	}
	if err := writeKey(leaf.keyPath(), key); err != nil {
		return "", err
	}
	return reason, nil
}
