package v1alpha1

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/MatchaScript/nanok8s/internal/version"
)

// Validate returns an aggregated error describing every problem found in c,
// or nil if the configuration is acceptable. Defaults should be applied
// before calling Validate.
func Validate(c *NanoK8sConfig) error {
	var errs []error

	if c.APIVersion != APIVersion {
		errs = append(errs, fmt.Errorf("apiVersion must be %q, got %q", APIVersion, c.APIVersion))
	}
	if c.Kind != Kind {
		errs = append(errs, fmt.Errorf("kind must be %q, got %q", Kind, c.Kind))
	}

	if c.Spec.KubernetesVersion != version.KubernetesVersion {
		errs = append(errs, fmt.Errorf(
			"spec.kubernetesVersion %q does not match this nanok8s binary (built for %q); "+
				"nanok8s minor versions are pinned 1:1 to kubelet",
			c.Spec.KubernetesVersion, version.KubernetesVersion))
	}

	cp := c.Spec.ControlPlane
	if cp.Mode != ControlPlaneModeSingle {
		errs = append(errs, fmt.Errorf("spec.controlPlane.mode must be %q in v0", ControlPlaneModeSingle))
	}
	if cp.AdvertiseAddress == "" {
		errs = append(errs, errors.New("spec.controlPlane.advertiseAddress is required"))
	} else if ip := net.ParseIP(cp.AdvertiseAddress); ip == nil {
		errs = append(errs, fmt.Errorf("spec.controlPlane.advertiseAddress %q is not a valid IP", cp.AdvertiseAddress))
	}
	if cp.BindPort < 1 || cp.BindPort > 65535 {
		errs = append(errs, fmt.Errorf("spec.controlPlane.bindPort %d out of range", cp.BindPort))
	}
	if _, _, err := net.ParseCIDR(cp.ServiceSubnet); err != nil {
		errs = append(errs, fmt.Errorf("spec.controlPlane.serviceSubnet: %w", err))
	}
	if _, _, err := net.ParseCIDR(cp.PodSubnet); err != nil {
		errs = append(errs, fmt.Errorf("spec.controlPlane.podSubnet: %w", err))
	}
	if ip := net.ParseIP(cp.ClusterDNS); ip == nil {
		errs = append(errs, fmt.Errorf("spec.controlPlane.clusterDNS %q is not a valid IP", cp.ClusterDNS))
	}

	rt := c.Spec.Runtime
	if !strings.HasPrefix(rt.CRISocket, "unix://") {
		errs = append(errs, fmt.Errorf("spec.runtime.criSocket must start with unix://, got %q", rt.CRISocket))
	}
	switch rt.CgroupDriver {
	case CgroupDriverSystemd, CgroupDriverCgroupfs:
	default:
		errs = append(errs, fmt.Errorf("spec.runtime.cgroupDriver must be systemd or cgroupfs, got %q", rt.CgroupDriver))
	}

	certs := c.Spec.Certificates
	if !certs.SelfSigned {
		errs = append(errs, errors.New("spec.certificates.selfSigned=false is not supported in v0"))
	}
	if certs.CAValidityDays <= 0 {
		errs = append(errs, errors.New("spec.certificates.caValidityDays must be > 0"))
	}
	if certs.LeafValidityDays <= 0 {
		errs = append(errs, errors.New("spec.certificates.leafValidityDays must be > 0"))
	}

	return errors.Join(errs...)
}
