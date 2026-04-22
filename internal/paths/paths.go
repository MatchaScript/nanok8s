// Package paths centralises every filesystem location nanok8s reads or writes.
// Having one source of truth here makes it straightforward to relocate trees
// for tests (override these vars before calling into other packages).
package paths

const (
	// ConfigDir holds user-facing nanok8s configuration.
	ConfigDir  = "/etc/nanok8s"
	ConfigFile = ConfigDir + "/config.yaml"

	// KubernetesDir is the standard kubelet-facing tree.
	KubernetesDir    = "/etc/kubernetes"
	PKIDir           = KubernetesDir + "/pki"
	EtcdPKIDir       = PKIDir + "/etcd"
	ManifestsDir     = KubernetesDir + "/manifests"
	KubeconfigDir    = KubernetesDir
	AdminKubeconfig  = KubeconfigDir + "/admin.conf"
	KubeletKubeconfig = KubeconfigDir + "/kubelet.conf"
	CMKubeconfig     = KubeconfigDir + "/controller-manager.conf"
	SchedKubeconfig  = KubeconfigDir + "/scheduler.conf"

	// KubeletDir is kubelet's own state directory.
	KubeletDir        = "/var/lib/kubelet"
	KubeletConfigFile = KubeletDir + "/config.yaml"
	KubeletFlagsEnv   = "/etc/sysconfig/kubelet"

	// RuntimeDir holds tmpfs-backed markers that must not survive reboot.
	RuntimeDir          = "/run/nanok8s"
	BootstrapDoneMarker = RuntimeDir + "/bootstrap.done"
)
