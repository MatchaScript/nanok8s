package staticpod

// Minimal subset of the core/v1 Pod schema. Hand-rolled to avoid pulling in
// k8s.io/api. json tags use the same names the real types use, so kubelet
// reads the resulting YAML identically to a k8s-generated manifest.

type pod struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   objectMeta `json:"metadata"`
	Spec       podSpec    `json:"spec"`
}

type objectMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type podSpec struct {
	HostNetwork       bool        `json:"hostNetwork,omitempty"`
	PriorityClassName string      `json:"priorityClassName,omitempty"`
	Containers        []container `json:"containers"`
	Volumes           []volume    `json:"volumes,omitempty"`
}

type container struct {
	Name          string        `json:"name"`
	Image         string        `json:"image"`
	Command       []string      `json:"command,omitempty"`
	Resources     resourceReqs  `json:"resources,omitempty"`
	VolumeMounts  []volumeMount `json:"volumeMounts,omitempty"`
	LivenessProbe *probe        `json:"livenessProbe,omitempty"`
	StartupProbe  *probe        `json:"startupProbe,omitempty"`
}

type resourceReqs struct {
	Requests map[string]string `json:"requests,omitempty"`
}

type volumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

type probe struct {
	HTTPGet             *httpGetAction `json:"httpGet,omitempty"`
	InitialDelaySeconds int32          `json:"initialDelaySeconds,omitempty"`
	PeriodSeconds       int32          `json:"periodSeconds,omitempty"`
	TimeoutSeconds      int32          `json:"timeoutSeconds,omitempty"`
	FailureThreshold    int32          `json:"failureThreshold,omitempty"`
}

type httpGetAction struct {
	Host   string `json:"host,omitempty"`
	Path   string `json:"path"`
	Port   int32  `json:"port"`
	Scheme string `json:"scheme,omitempty"`
}

type volume struct {
	Name     string                `json:"name"`
	HostPath *hostPathVolumeSource `json:"hostPath,omitempty"`
}

type hostPathVolumeSource struct {
	Path string `json:"path"`
	Type string `json:"type,omitempty"`
}
