package v1alpha1

import "github.com/MatchaScript/nanok8s/internal/version"

const (
	DefaultBindPort         int32 = 6443
	DefaultServiceSubnet          = "10.96.0.0/12"
	DefaultPodSubnet              = "10.244.0.0/16"
	DefaultClusterDNS             = "10.96.0.10"
	DefaultCRISocket              = "unix:///var/run/crio/crio.sock"
	DefaultCAValidityDays   int32 = 3650
	DefaultLeafValidityDays int32 = 3650
)

// SetDefaults fills zero-valued fields with the project defaults.
// It mutates the argument in place.
func SetDefaults(c *NanoK8sConfig) {
	if c.APIVersion == "" {
		c.APIVersion = APIVersion
	}
	if c.Kind == "" {
		c.Kind = Kind
	}

	if c.Spec.KubernetesVersion == "" {
		c.Spec.KubernetesVersion = version.KubernetesVersion
	}

	cp := &c.Spec.ControlPlane
	if cp.Mode == "" {
		cp.Mode = ControlPlaneModeSingle
	}
	if cp.BindPort == 0 {
		cp.BindPort = DefaultBindPort
	}
	if cp.ServiceSubnet == "" {
		cp.ServiceSubnet = DefaultServiceSubnet
	}
	if cp.PodSubnet == "" {
		cp.PodSubnet = DefaultPodSubnet
	}
	if cp.ClusterDNS == "" {
		cp.ClusterDNS = DefaultClusterDNS
	}

	rt := &c.Spec.Runtime
	if rt.CRISocket == "" {
		rt.CRISocket = DefaultCRISocket
	}
	if rt.CgroupDriver == "" {
		rt.CgroupDriver = CgroupDriverSystemd
	}

	certs := &c.Spec.Certificates
	if certs.CAValidityDays == 0 {
		certs.CAValidityDays = DefaultCAValidityDays
	}
	if certs.LeafValidityDays == 0 {
		certs.LeafValidityDays = DefaultLeafValidityDays
	}
}

// NewDefault returns a NanoK8sConfig with all defaults applied and a
// placeholder advertiseAddress. Used by `config print-defaults`.
func NewDefault() *NanoK8sConfig {
	c := &NanoK8sConfig{
		Metadata: ObjectMeta{Name: "local"},
		Spec: NanoK8sConfigSpec{
			ControlPlane: ControlPlaneSpec{
				AdvertiseAddress: "0.0.0.0",
			},
			Certificates: CertificatesSpec{
				SelfSigned: true,
				ExtraSANs:  []string{"127.0.0.1", "localhost"},
			},
		},
	}
	SetDefaults(c)
	return c
}
