// Package v1alpha1 defines the NanoK8sConfig API consumed by `nanok8s bootstrap`
// and the oneshot `nanok8s.service` boot flow. The shape intentionally mirrors
// kubeadm InitConfiguration fields where equivalents exist, so mapping is
// straightforward.
package v1alpha1

import corev1 "k8s.io/api/core/v1"

const (
	GroupName  = "bootstrap.nanok8s.io"
	Version    = "v1alpha1"
	APIVersion = GroupName + "/" + Version
	Kind       = "NanoK8sConfig"
)

type TypeMeta struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

type ObjectMeta struct {
	Name string `json:"name,omitempty"`
}

type NanoK8sConfig struct {
	TypeMeta `json:",inline"`
	Metadata ObjectMeta        `json:"metadata,omitempty"`
	Spec     NanoK8sConfigSpec `json:"spec"`
}

type NanoK8sConfigSpec struct {
	ControlPlane     ControlPlaneSpec     `json:"controlPlane"`
	Runtime          RuntimeSpec          `json:"runtime,omitempty"`
	Certificates     CertificatesSpec     `json:"certificates,omitempty"`
	NodeRegistration NodeRegistrationSpec `json:"nodeRegistration,omitempty"`
}

// NodeRegistrationSpec mirrors kubeadm's InitConfiguration.NodeRegistration
// fields that nanok8s currently exposes.
type NodeRegistrationSpec struct {
	// Taints applied to the node after apiserver becomes reachable.
	// nil  → use the default ([node-role.kubernetes.io/control-plane:NoSchedule]).
	// []   → no taints (workloads scheduled onto the control-plane node).
	// else → exactly the listed taints.
	Taints []corev1.Taint `json:"taints,omitempty"`
}

type ControlPlaneMode string

const (
	ControlPlaneModeSingle ControlPlaneMode = "single"
)

type ControlPlaneSpec struct {
	Mode             ControlPlaneMode `json:"mode,omitempty"`
	AdvertiseAddress string           `json:"advertiseAddress"`
	BindPort         int32            `json:"bindPort,omitempty"`
	ServiceSubnet    string           `json:"serviceSubnet,omitempty"`
	PodSubnet        string           `json:"podSubnet,omitempty"`
	ClusterDNS       string           `json:"clusterDNS,omitempty"`
}

type CgroupDriver string

const (
	CgroupDriverSystemd  CgroupDriver = "systemd"
	CgroupDriverCgroupfs CgroupDriver = "cgroupfs"
)

type RuntimeSpec struct {
	CRISocket    string       `json:"criSocket,omitempty"`
	CgroupDriver CgroupDriver `json:"cgroupDriver,omitempty"`
}

type CertificatesSpec struct {
	SelfSigned       bool     `json:"selfSigned,omitempty"`
	CAValidityDays   int32    `json:"caValidityDays,omitempty"`
	LeafValidityDays int32    `json:"leafValidityDays,omitempty"`
	ExtraSANs        []string `json:"extraSANs,omitempty"`
}
