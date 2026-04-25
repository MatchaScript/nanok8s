#!/usr/bin/env bash
# Provision a fresh Ubuntu runner with everything nanok8s needs to run:
# CRI-O, a raw kubelet binary matching nanok8s's pinned minor, kubectl,
# CNI plugins, crictl, and kernel prerequisites (swap off, br_netfilter,
# sysctls). Idempotent: safe to rerun on an already-configured host.
#
# Called once per E2E run (via e2e.sh); not intended to be invoked on its
# own outside CI.

set -Eeuo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./lib.sh
source "$SCRIPT_DIR/lib.sh"

REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

install_packages() {
    log_step "Installing base packages"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -qq -y --no-install-recommends \
        socat conntrack ipset ethtool iptables jq curl ca-certificates gpg \
        iproute2 ipvsadm
}

disable_swap() {
    log_step "Disabling swap"
    swapoff -a || true
    sed -i -E 's|^([^#].*\s+swap\s+.*)$|# \1|' /etc/fstab || true
}

load_kernel_modules() {
    log_step "Loading br_netfilter + overlay"
    cat >/etc/modules-load.d/nanok8s.conf <<'EOF'
overlay
br_netfilter
EOF
    modprobe overlay
    modprobe br_netfilter
}

set_sysctls() {
    log_step "Setting kube-required sysctls"
    cat >/etc/sysctl.d/99-nanok8s.conf <<'EOF'
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF
    sysctl --system >/dev/null
}

install_cni_plugins() {
    log_step "Installing CNI plugins $CNI_PLUGINS_VERSION"
    local url="https://github.com/containernetworking/plugins/releases/download/$CNI_PLUGINS_VERSION/cni-plugins-linux-amd64-$CNI_PLUGINS_VERSION.tgz"
    mkdir -p /opt/cni/bin
    curl -fsSL "$url" | tar -xz -C /opt/cni/bin
}

install_crio() {
    log_step "Installing CRI-O $CRIO_VERSION"
    # CRI-O packages moved off pkgs.k8s.io in 2025 and now live on the
    # openSUSE Build Service under the upstream-maintained isv:/cri-o:/ project.
    local base="https://download.opensuse.org/repositories/isv:/cri-o:/stable:/$CRIO_VERSION/deb"
    mkdir -p /etc/apt/keyrings
    curl -fsSL "$base/Release.key" |
        gpg --dearmor --yes -o /etc/apt/keyrings/cri-o-apt-keyring.gpg
    cat >/etc/apt/sources.list.d/cri-o.list <<EOF
deb [signed-by=/etc/apt/keyrings/cri-o-apt-keyring.gpg] $base/ /
EOF
    apt-get update -qq
    apt-get install -qq -y --no-install-recommends cri-o
    systemctl enable --now "$CRIO_SERVICE"
    # Fail fast if the socket never appears; nanok8s will otherwise wait
    # 3 minutes on /readyz and we'd rather surface the root cause here.
    retry 20 1 test -S /var/run/crio/crio.sock || die "crio.sock never appeared"
}

install_kubelet_kubectl() {
    log_step "Installing kubelet + kubectl $KUBELET_VERSION"
    curl -fsSL "https://dl.k8s.io/release/$KUBELET_VERSION/bin/linux/amd64/kubelet" \
        -o /usr/local/bin/kubelet
    curl -fsSL "https://dl.k8s.io/release/$KUBECTL_VERSION/bin/linux/amd64/kubectl" \
        -o /usr/local/bin/kubectl
    chmod +x /usr/local/bin/kubelet /usr/local/bin/kubectl

    # kubelet unit matching the flags/env nanok8s's kubeadm phases expect.
    # Aligned with https://github.com/kubernetes/release/blob/master/cmd/krel/templates/latest/kubelet/kubelet.service
    cat >"$KUBELET_SERVICE_UNIT" <<'EOF'
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=https://kubernetes.io/docs/
Wants=network-online.target
After=network-online.target

[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
EnvironmentFile=-/etc/default/kubelet
ExecStart=/usr/local/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
}

install_crictl() {
    log_step "Installing crictl $CRICTL_VERSION"
    local url="https://github.com/kubernetes-sigs/cri-tools/releases/download/$CRICTL_VERSION/crictl-$CRICTL_VERSION-linux-amd64.tar.gz"
    curl -fsSL "$url" | tar -xz -C /usr/local/bin
    # crictl picks the CRI-O socket from /etc/crictl.yaml
    cat >/etc/crictl.yaml <<'EOF'
runtime-endpoint: unix:///var/run/crio/crio.sock
image-endpoint: unix:///var/run/crio/crio.sock
timeout: 10
EOF
}

install_nanok8s_service() {
    log_step "Installing nanok8s.service unit + config"
    install -m 0644 "$REPO_ROOT/packaging/systemd/nanok8s.service" "$NANOK8S_SERVICE_UNIT"
    mkdir -p /etc/nanok8s
    # Seed config.yaml from `nanok8s config print-defaults` and override
    # advertiseAddress with the primary non-loopback IP so the apiserver
    # actually accepts the SAN.
    local primary_ip
    primary_ip=$(hostname -I | awk '{print $1}')
    "$NANOK8S_BIN" config print-defaults \
        | sed -E "s|^([[:space:]]+advertiseAddress:[[:space:]]).*$|\1$primary_ip|" \
        >"$NANOK8S_CONFIG"
    # Verify the rewrite landed: leaving advertiseAddress=0.0.0.0 would
    # later fail kubelet bootstrapping with a SAN mismatch.
    if ! grep -qE "advertiseAddress: $primary_ip" "$NANOK8S_CONFIG"; then
        die "failed to rewrite advertiseAddress in $NANOK8S_CONFIG"
    fi
    systemctl daemon-reload
    log_info "nanok8s config written (advertiseAddress=$primary_ip)"
}

# ensure_clean_start removes any artefacts from a prior failed run so the
# suite can be re-run on the same host without rebooting. Mirrors what
# `nanok8s reset` does, for idempotence of this setup script itself.
ensure_clean_start() {
    log_step "Ensuring clean starting state"
    if [[ -x "$NANOK8S_BIN" ]] && [[ -d /etc/kubernetes ]]; then
        "$NANOK8S_BIN" reset --yes || true
    fi
    rm -rf /etc/kubernetes /var/lib/etcd /var/lib/kubelet /var/lib/nanok8s
}

setup() {
    install_packages
    disable_swap
    load_kernel_modules
    set_sysctls
    install_cni_plugins
    install_crio
    install_kubelet_kubectl
    install_crictl
    install_nanok8s_service
    ensure_clean_start
    log_info "Setup complete"
}

# Allow sourcing for tests + running standalone.
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    setup
fi
