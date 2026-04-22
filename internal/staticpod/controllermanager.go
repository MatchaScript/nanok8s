package staticpod

import (
	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
	"github.com/MatchaScript/nanok8s/internal/paths"
	"github.com/MatchaScript/nanok8s/internal/version"
)

func controllerManagerPod(cfg *v1alpha1.NanoK8sConfig) *pod {
	return &pod{
		APIVersion: "v1",
		Kind:       "Pod",
		Metadata: objectMeta{
			Name:      "kube-controller-manager",
			Namespace: "kube-system",
			Labels:    map[string]string{"component": "kube-controller-manager", "tier": "control-plane"},
		},
		Spec: podSpec{
			HostNetwork:       true,
			PriorityClassName: "system-node-critical",
			Containers: []container{{
				Name:  "kube-controller-manager",
				Image: version.ImageFor("kube-controller-manager"),
				Command: []string{
					"kube-controller-manager",
					"--allocate-node-cidrs=true",
					"--authentication-kubeconfig=" + paths.CMKubeconfig,
					"--authorization-kubeconfig=" + paths.CMKubeconfig,
					"--bind-address=127.0.0.1",
					"--client-ca-file=" + paths.PKIDir + "/ca.crt",
					"--cluster-cidr=" + cfg.Spec.ControlPlane.PodSubnet,
					"--cluster-name=kubernetes",
					"--cluster-signing-cert-file=" + paths.PKIDir + "/ca.crt",
					"--cluster-signing-key-file=" + paths.PKIDir + "/ca.key",
					"--controllers=*,bootstrapsigner,tokencleaner",
					"--kubeconfig=" + paths.CMKubeconfig,
					"--leader-elect=true",
					"--requestheader-client-ca-file=" + paths.PKIDir + "/front-proxy-ca.crt",
					"--root-ca-file=" + paths.PKIDir + "/ca.crt",
					"--service-account-private-key-file=" + paths.PKIDir + "/sa.key",
					"--service-cluster-ip-range=" + cfg.Spec.ControlPlane.ServiceSubnet,
					"--use-service-account-credentials=true",
				},
				Resources: resourceReqs{Requests: map[string]string{"cpu": "200m"}},
				VolumeMounts: []volumeMount{
					{Name: "k8s-certs", MountPath: paths.PKIDir, ReadOnly: true},
					{Name: "ca-certs", MountPath: "/etc/ssl/certs", ReadOnly: true},
					{Name: "kubeconfig", MountPath: paths.CMKubeconfig, ReadOnly: true},
				},
				LivenessProbe: httpProbe("127.0.0.1", "/healthz", 10257, "HTTPS", 10, 10, 15, 8),
				StartupProbe:  httpProbe("127.0.0.1", "/healthz", 10257, "HTTPS", 10, 10, 15, 24),
			}},
			Volumes: []volume{
				{Name: "k8s-certs", HostPath: &hostPathVolumeSource{Path: paths.PKIDir, Type: "DirectoryOrCreate"}},
				{Name: "ca-certs", HostPath: &hostPathVolumeSource{Path: "/etc/ssl/certs", Type: "DirectoryOrCreate"}},
				{Name: "kubeconfig", HostPath: &hostPathVolumeSource{Path: paths.CMKubeconfig, Type: "FileOrCreate"}},
			},
		},
	}
}
