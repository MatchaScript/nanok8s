package staticpod

import (
	"fmt"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
	"github.com/MatchaScript/nanok8s/internal/paths"
	"github.com/MatchaScript/nanok8s/internal/version"
)

func apiserverPod(cfg *v1alpha1.NanoK8sConfig) *pod {
	advertise := cfg.Spec.ControlPlane.AdvertiseAddress
	port := cfg.Spec.ControlPlane.BindPort
	return &pod{
		APIVersion: "v1",
		Kind:       "Pod",
		Metadata: objectMeta{
			Name:      "kube-apiserver",
			Namespace: "kube-system",
			Labels:    map[string]string{"component": "kube-apiserver", "tier": "control-plane"},
		},
		Spec: podSpec{
			HostNetwork:       true,
			PriorityClassName: "system-node-critical",
			Containers: []container{{
				Name:  "kube-apiserver",
				Image: version.ImageFor("kube-apiserver"),
				Command: []string{
					"kube-apiserver",
					"--advertise-address=" + advertise,
					"--allow-privileged=true",
					"--authorization-mode=Node,RBAC",
					"--client-ca-file=" + paths.PKIDir + "/ca.crt",
					"--enable-admission-plugins=NodeRestriction",
					"--enable-bootstrap-token-auth=true",
					"--etcd-cafile=" + paths.EtcdPKIDir + "/ca.crt",
					"--etcd-certfile=" + paths.PKIDir + "/apiserver-etcd-client.crt",
					"--etcd-keyfile=" + paths.PKIDir + "/apiserver-etcd-client.key",
					fmt.Sprintf("--etcd-servers=https://%s:2379", advertise),
					"--kubelet-client-certificate=" + paths.PKIDir + "/apiserver-kubelet-client.crt",
					"--kubelet-client-key=" + paths.PKIDir + "/apiserver-kubelet-client.key",
					"--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname",
					"--proxy-client-cert-file=" + paths.PKIDir + "/front-proxy-client.crt",
					"--proxy-client-key-file=" + paths.PKIDir + "/front-proxy-client.key",
					"--requestheader-allowed-names=front-proxy-client",
					"--requestheader-client-ca-file=" + paths.PKIDir + "/front-proxy-ca.crt",
					"--requestheader-extra-headers-prefix=X-Remote-Extra-",
					"--requestheader-group-headers=X-Remote-Group",
					"--requestheader-username-headers=X-Remote-User",
					fmt.Sprintf("--secure-port=%d", port),
					"--service-account-issuer=https://kubernetes.default.svc.cluster.local",
					"--service-account-key-file=" + paths.PKIDir + "/sa.pub",
					"--service-account-signing-key-file=" + paths.PKIDir + "/sa.key",
					"--service-cluster-ip-range=" + cfg.Spec.ControlPlane.ServiceSubnet,
					"--tls-cert-file=" + paths.PKIDir + "/apiserver.crt",
					"--tls-private-key-file=" + paths.PKIDir + "/apiserver.key",
				},
				Resources: resourceReqs{Requests: map[string]string{"cpu": "250m"}},
				VolumeMounts: []volumeMount{
					{Name: "k8s-certs", MountPath: paths.PKIDir, ReadOnly: true},
					{Name: "ca-certs", MountPath: "/etc/ssl/certs", ReadOnly: true},
				},
				LivenessProbe: httpProbe(advertise, "/livez", port, "HTTPS", 10, 10, 15, 8),
				StartupProbe:  httpProbe(advertise, "/livez", port, "HTTPS", 10, 10, 15, 24),
			}},
			Volumes: []volume{
				{Name: "k8s-certs", HostPath: &hostPathVolumeSource{Path: paths.PKIDir, Type: "DirectoryOrCreate"}},
				{Name: "ca-certs", HostPath: &hostPathVolumeSource{Path: "/etc/ssl/certs", Type: "DirectoryOrCreate"}},
			},
		},
	}
}
