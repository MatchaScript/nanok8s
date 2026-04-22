package staticpod

import (
	"github.com/MatchaScript/nanok8s/internal/paths"
	"github.com/MatchaScript/nanok8s/internal/version"
)

func schedulerPod() *pod {
	return &pod{
		APIVersion: "v1",
		Kind:       "Pod",
		Metadata: objectMeta{
			Name:      "kube-scheduler",
			Namespace: "kube-system",
			Labels:    map[string]string{"component": "kube-scheduler", "tier": "control-plane"},
		},
		Spec: podSpec{
			HostNetwork:       true,
			PriorityClassName: "system-node-critical",
			Containers: []container{{
				Name:  "kube-scheduler",
				Image: version.ImageFor("kube-scheduler"),
				Command: []string{
					"kube-scheduler",
					"--authentication-kubeconfig=" + paths.SchedKubeconfig,
					"--authorization-kubeconfig=" + paths.SchedKubeconfig,
					"--bind-address=127.0.0.1",
					"--kubeconfig=" + paths.SchedKubeconfig,
					"--leader-elect=true",
				},
				Resources: resourceReqs{Requests: map[string]string{"cpu": "100m"}},
				VolumeMounts: []volumeMount{
					{Name: "kubeconfig", MountPath: paths.SchedKubeconfig, ReadOnly: true},
				},
				LivenessProbe: httpProbe("127.0.0.1", "/healthz", 10259, "HTTPS", 10, 10, 15, 8),
				StartupProbe:  httpProbe("127.0.0.1", "/healthz", 10259, "HTTPS", 10, 10, 15, 24),
			}},
			Volumes: []volume{
				{Name: "kubeconfig", HostPath: &hostPathVolumeSource{Path: paths.SchedKubeconfig, Type: "FileOrCreate"}},
			},
		},
	}
}
