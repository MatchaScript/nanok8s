// Package paths centralises every filesystem location nanok8s reads or writes.
// Having one source of truth here makes it straightforward to relocate trees
// for tests (override these vars before calling into other packages).
package paths

const (
	// ConfigDir holds user-facing nanok8s configuration.
	ConfigDir  = "/etc/nanok8s"
	ConfigFile = ConfigDir + "/config.yaml"

	// KubernetesDir is the standard kubelet-facing tree.
	KubernetesDir     = "/etc/kubernetes"
	PKIDir            = KubernetesDir + "/pki"
	EtcdPKIDir        = PKIDir + "/etcd"
	ManifestsDir      = KubernetesDir + "/manifests"
	KubeconfigDir     = KubernetesDir
	AdminKubeconfig   = KubeconfigDir + "/admin.conf"
	KubeletKubeconfig = KubeconfigDir + "/kubelet.conf"
	CMKubeconfig      = KubeconfigDir + "/controller-manager.conf"
	SchedKubeconfig   = KubeconfigDir + "/scheduler.conf"

	// KubeletDir is kubelet's own state directory.
	KubeletDir          = "/var/lib/kubelet"
	KubeletConfigFile   = KubeletDir + "/config.yaml"
	KubeletFlagsEnvFile = KubeletDir + "/kubeadm-flags.env"

	// EtcdDataDir is the etcd static pod's data directory. Snapshotted by
	// nanok8s on every boot before kubelet brings etcd back up.
	EtcdDataDir = "/var/lib/etcd"

	// NanoK8sVarDir holds all nanok8s-owned mutable state (state files,
	// backups). /var survives bootc rollback; we explicitly version
	// sub-trees ourselves so rollback + restore is decoupled from /var.
	NanoK8sVarDir = "/var/lib/nanok8s"
	StateDir      = NanoK8sVarDir + "/state"
	BackupsDir    = NanoK8sVarDir + "/backups"

	// State files (under StateDir).
	LastBootFile  = StateDir + "/last-boot.json"
	LastEventFile = StateDir + "/last-event"

	// RestoreMarker is touched by the greenboot red.d hook on a rollback
	// boot to request that the next boot restore a backup for the
	// (post-rollback) deployment.
	RestoreMarker = BackupsDir + "/restore"
)
