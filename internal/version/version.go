// Package version exposes build-time constants. Values are overridden via
// -ldflags "-X github.com/MatchaScript/nanok8s/internal/version.<Name>=<value>"
// during release builds.
package version

// KubernetesVersion is the single k8s minor this nanok8s build targets.
// nanok8s minor == kubelet minor is a hard constraint; Validate() rejects
// any config that disagrees.
var KubernetesVersion = "v1.35.0"

// GitCommit is the commit hash of the nanok8s source tree at build time.
var GitCommit = "unknown"

// BuildDate is the RFC3339 build timestamp.
var BuildDate = "unknown"

// Component image pins for this minor. Bumped together with KubernetesVersion.
var (
	EtcdImage    = "registry.k8s.io/etcd:3.5.21-0"
	PauseImage   = "registry.k8s.io/pause:3.10"
	CoreDNSImage = "registry.k8s.io/coredns/coredns:v1.11.3"
)

// ImageFor returns the image reference for a core control-plane component
// (kube-apiserver, kube-controller-manager, kube-scheduler, kube-proxy).
// Tag tracks KubernetesVersion; a single nanok8s binary always points at
// exactly one minor.
func ImageFor(component string) string {
	return "registry.k8s.io/" + component + ":" + KubernetesVersion
}
