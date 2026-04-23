package v1alpha1

import corev1 "k8s.io/api/core/v1"

const (
	DefaultBindPort         int32 = 6443
	DefaultServiceSubnet          = "10.96.0.0/12"
	DefaultPodSubnet              = "10.244.0.0/16"
	DefaultClusterDNS             = "10.96.0.10"
	DefaultCRISocket              = "unix:///var/run/crio/crio.sock"
	DefaultCAValidityDays   int32 = 3650
	DefaultLeafValidityDays int32 = 3650

	// ControlPlaneTaintKey is the kubeadm-standard node taint applied to
	// control-plane nodes. Kept in sync with
	// kubeadmconstants.LabelNodeRoleControlPlane.
	ControlPlaneTaintKey = "node-role.kubernetes.io/control-plane"
)

// DefaultControlPlaneTaint is the single-taint default used when the user
// leaves spec.nodeRegistration.taints unset. Matches kubeadm's
// DefaultedStaticInitConfiguration() output.
var DefaultControlPlaneTaint = corev1.Taint{
	Key:    ControlPlaneTaintKey,
	Effect: corev1.TaintEffectNoSchedule,
}

// SetDefaults fills zero-valued fields with the project defaults.
// It mutates the argument in place.
func SetDefaults(c *NanoK8sConfig) {
	if c.APIVersion == "" {
		c.APIVersion = APIVersion
	}
	if c.Kind == "" {
		c.Kind = Kind
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

	// Taints: nil => default, [] => explicit "no taints". SetDefaults only
	// substitutes when the user did not set the field at all (nil slice).
	nr := &c.Spec.NodeRegistration
	if nr.Taints == nil {
		nr.Taints = []corev1.Taint{DefaultControlPlaneTaint}
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
