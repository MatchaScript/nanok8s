package staticpod

import (
	"fmt"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
	"github.com/MatchaScript/nanok8s/internal/paths"
	"github.com/MatchaScript/nanok8s/internal/version"
)

func etcdPod(cfg *v1alpha1.NanoK8sConfig, nodeName string) *pod {
	advertise := cfg.Spec.ControlPlane.AdvertiseAddress
	return &pod{
		APIVersion: "v1",
		Kind:       "Pod",
		Metadata: objectMeta{
			Name:      "etcd-" + nodeName,
			Namespace: "kube-system",
			Labels:    map[string]string{"component": "etcd", "tier": "control-plane"},
		},
		Spec: podSpec{
			HostNetwork:       true,
			PriorityClassName: "system-node-critical",
			Containers: []container{{
				Name:  "etcd",
				Image: version.EtcdImage,
				Command: []string{
					"etcd",
					"--name=" + nodeName,
					"--data-dir=/var/lib/etcd",
					fmt.Sprintf("--listen-client-urls=https://127.0.0.1:2379,https://%s:2379", advertise),
					fmt.Sprintf("--advertise-client-urls=https://%s:2379", advertise),
					fmt.Sprintf("--listen-peer-urls=https://%s:2380", advertise),
					fmt.Sprintf("--initial-advertise-peer-urls=https://%s:2380", advertise),
					fmt.Sprintf("--initial-cluster=%s=https://%s:2380", nodeName, advertise),
					"--initial-cluster-state=new",
					"--listen-metrics-urls=http://127.0.0.1:2381",
					"--cert-file=" + paths.EtcdPKIDir + "/server.crt",
					"--key-file=" + paths.EtcdPKIDir + "/server.key",
					"--client-cert-auth=true",
					"--trusted-ca-file=" + paths.EtcdPKIDir + "/ca.crt",
					"--peer-cert-file=" + paths.EtcdPKIDir + "/peer.crt",
					"--peer-key-file=" + paths.EtcdPKIDir + "/peer.key",
					"--peer-client-cert-auth=true",
					"--peer-trusted-ca-file=" + paths.EtcdPKIDir + "/ca.crt",
					"--snapshot-count=10000",
				},
				Resources: resourceReqs{Requests: map[string]string{"cpu": "100m", "memory": "100Mi"}},
				VolumeMounts: []volumeMount{
					{Name: "etcd-certs", MountPath: paths.EtcdPKIDir},
					{Name: "etcd-data", MountPath: "/var/lib/etcd"},
				},
				LivenessProbe: httpProbe("127.0.0.1", "/health", 2381, "HTTP", 10, 10, 15, 8),
				StartupProbe:  httpProbe("127.0.0.1", "/health", 2381, "HTTP", 10, 10, 15, 24),
			}},
			Volumes: []volume{
				{Name: "etcd-certs", HostPath: &hostPathVolumeSource{Path: paths.EtcdPKIDir, Type: "DirectoryOrCreate"}},
				{Name: "etcd-data", HostPath: &hostPathVolumeSource{Path: "/var/lib/etcd", Type: "DirectoryOrCreate"}},
			},
		},
	}
}

// httpProbe is a small helper shared across component pods.
func httpProbe(host, path string, port int32, scheme string, initial, period, timeout, failure int32) *probe {
	return &probe{
		HTTPGet:             &httpGetAction{Host: host, Path: path, Port: port, Scheme: scheme},
		InitialDelaySeconds: initial,
		PeriodSeconds:       period,
		TimeoutSeconds:      timeout,
		FailureThreshold:    failure,
	}
}
