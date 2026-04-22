# nanok8s

Minimal, single-node Kubernetes runtime for bootc-style edge deployments.
Upstream-Kubernetes positioned in the same niche as k0s; architecturally in
the same niche as MicroShift, minus the OpenShift-specific layers.

## What it does

`nanok8s` turns a bootc-provisioned host into a functional single-node
Kubernetes cluster by:

1. Generating the self-signed PKI, kubeconfig files, static pod manifests,
   and kubelet configuration that kubeadm would produce on a fresh node.
   The kubeadm Go phases are used as a library, not shelled out to.
2. Handing control to `kubelet`, which starts `etcd`, `kube-apiserver`,
   `kube-controller-manager`, and `kube-scheduler` as static pods on CRI-O.
3. Applying `CoreDNS` and `kube-proxy` once the apiserver is reachable.

What nanok8s does **not** do:

- **CNI**: deliberately out of scope. Install your own (Flannel, Calico,
  Cilium) after first boot.
- **Multi-node**: v0 targets a single node with the control plane and
  workloads colocated. Join support is not implemented.
- **Upgrades**: rely on bootc image swaps. The `nanok8s` binary, kubelet,
  CRI-O, and component image tags all come from the image; bumping the
  image bumps the cluster.

## Image contents

A bootc image using nanok8s ships, at minimum:

- `cri-o`
- `kubelet`
- `/usr/bin/nanok8s`
- `/usr/lib/systemd/system/nanok8s-apply.service`
- `/usr/lib/systemd/system/nanok8s-apply.timer`

and expects `/etc/nanok8s/config.yaml` to be present (baked into the image
or placed via cloud-init / ignition).

## Config

See `nanok8s config print-defaults` for a fully populated example. Minimum:

```yaml
apiVersion: bootstrap.nanok8s.io/v1alpha1
kind: NanoK8sConfig
metadata:
  name: local
spec:
  controlPlane:
    advertiseAddress: 192.168.10.10
  certificates:
    selfSigned: true
```

`nanok8s config validate` verifies a file before it is used.

## First-boot flow

On a freshly imaged host with `/etc/nanok8s/config.yaml` present:

```
# 1. Generate PKI, kubeconfigs, static pod manifests, kubelet config.
nanok8s bootstrap

# 2. Start kubelet; it picks up the static pod manifests.
systemctl enable --now kubelet.service

# 3. Once the apiserver is reachable, apply CoreDNS and kube-proxy.
nanok8s addons apply

# 4. Enable drift reconciliation (runs daily, survives reboots).
systemctl enable --now nanok8s-apply.timer

# 5. Install a CNI of your choice.
kubectl apply -f <flannel|calico|cilium manifest>
```

After this, reboot is zero-touch: kubelet brings the static pods back up
from on-disk manifests, and the timer converges any drift.

Step 1 is explicitly manual: operators write config once and trigger the
initial bootstrap themselves. Steps 2-4 will collapse into a single
`systemctl enable --now nanok8s` in a future release once the reconciler
is moved into a long-running service.

## Directory layout

`nanok8s` writes into kubeadm-compatible locations:

| Path | Purpose |
| ---- | ------- |
| `/etc/nanok8s/config.yaml` | `NanoK8sConfig` source |
| `/etc/kubernetes/pki/` | CA and leaf certificates |
| `/etc/kubernetes/{admin,controller-manager,scheduler,kubelet}.conf` | kubeconfigs |
| `/etc/kubernetes/manifests/` | static pod manifests |
| `/var/lib/kubelet/config.yaml` | kubelet configuration |
| `/var/lib/kubelet/kubeadm-flags.env` | kubelet flag environment |

## Commands

| Command | Purpose |
| ------- | ------- |
| `nanok8s bootstrap` | Generate pre-kubelet artifacts (manual, once) |
| `nanok8s apply` | Reconcile bootstrap artifacts + best-effort addons |
| `nanok8s addons apply` | Apply CoreDNS and kube-proxy to a running apiserver |
| `nanok8s config validate` | Validate `/etc/nanok8s/config.yaml` |
| `nanok8s config print-defaults` | Emit a fully-defaulted config as YAML |
| `nanok8s version` | Print build and target Kubernetes versions |

## Build

```
go build ./cmd/nanok8s
```

## Status

v0, not yet released. APIs and on-disk formats may break between commits.
