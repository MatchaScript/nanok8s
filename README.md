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
3. Bootstrapping the `kubeadm:cluster-admins` ClusterRoleBinding, marking
   the node as a control plane (labels + taints), and applying `CoreDNS`
   and `kube-proxy` once the apiserver is reachable.
4. On subsequent boots: snapshotting the on-disk state for rollback
   (ostree/bootc only), and restoring the last healthy backup when
   greenboot signals a rollback.

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
- `/usr/lib/systemd/system/nanok8s.service` (oneshot, `Before=kubelet.service`)
- `/etc/greenboot/check/required.d/40-nanok8s.sh`
- `/etc/greenboot/check/wanted.d/40-nanok8s-status.sh`
- `/etc/greenboot/red.d/40-nanok8s-pre-rollback.sh`

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

Notable optional fields:

- `spec.nodeRegistration.taints` — taints applied to the node once the
  apiserver is reachable. Unset (nil) inherits the kubeadm standard
  `node-role.kubernetes.io/control-plane:NoSchedule`. Set to `[]` to
  schedule workloads onto the control-plane node.

`nanok8s config validate` verifies a file before it is used.

## First-boot flow

On a freshly imaged host with `/etc/nanok8s/config.yaml` present:

```
# 1. Generate PKI, kubeconfigs, static pod manifests, kubelet config.
nanok8s bootstrap

# 2. Enable the oneshot service. It runs Before=kubelet.service on every
#    boot, restores a backup if greenboot requested one, reconciles
#    bootstrap artifacts, starts kubelet, waits for the control plane to
#    come up, marks the node, and applies CoreDNS + kube-proxy.
systemctl enable --now nanok8s.service

# 3. Install a CNI of your choice.
kubectl apply -f <flannel|calico|cilium manifest>
```

After this, reboot is zero-touch: `nanok8s.service` re-runs, kubelet
brings the static pods back up, and backup/restore handles rollback
automatically on ostree/bootc systems.

## Directory layout

`nanok8s` writes into kubeadm-compatible locations, plus its own
state/backups tree under `/var/lib/nanok8s`:

| Path | Purpose |
| ---- | ------- |
| `/etc/nanok8s/config.yaml` | `NanoK8sConfig` source |
| `/etc/kubernetes/pki/` | CA and leaf certificates |
| `/etc/kubernetes/{admin,super-admin,controller-manager,scheduler,kubelet}.conf` | kubeconfigs |
| `/etc/kubernetes/manifests/` | static pod manifests |
| `/var/lib/kubelet/config.yaml` | kubelet configuration |
| `/var/lib/kubelet/kubeadm-flags.env` | kubelet flag environment |
| `/var/lib/etcd/` | etcd data |
| `/var/lib/nanok8s/state/last-boot.json` | metadata of the last healthy boot |
| `/var/lib/nanok8s/state/last-event` | human-readable lifecycle event (MOTD) |
| `/var/lib/nanok8s/backups/<deployID>_<bootID>/` | per-deployment backups (ostree only) |

## Commands

| Command | Purpose |
| ------- | ------- |
| `nanok8s bootstrap` | Generate pre-kubelet artifacts (manual, once per install) |
| `nanok8s reset --yes` | Stop kubelet, remove CRI containers, wipe /etc/kubernetes + /var/lib/{etcd,kubelet,nanok8s}, delete CNI interfaces, flush iptables/ipvs |
| `nanok8s config validate` | Validate `/etc/nanok8s/config.yaml` |
| `nanok8s config print-defaults` | Emit a fully-defaulted config as YAML |
| `nanok8s version` | Print build and target Kubernetes versions |

`nanok8s.service` internally invokes a hidden `nanok8s boot` subcommand;
operators should not call it directly.

## Build

```
go build ./cmd/nanok8s
```

## Status

v0, not yet released. APIs and on-disk formats may break between commits.
