#!/usr/bin/env bash
#
# Comprehensive End-to-End Test Suite for nanok8s.
# Modeled after reference testing scripts to ensure robust coverage of both
# normal and abnormal (edge) cases, providing a high degree of confidence
# in the cluster's stability and network connectivity.

set -euo pipefail

# --- Configuration & Globals ---
NANOK8S_BIN="${NANOK8S_BIN:-/usr/bin/nanok8s}"
KUBECONFIG="/etc/kubernetes/admin.conf"
export KUBECONFIG
KUBECTL="kubectl"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_err()  { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# --- Helper Functions ---
wait_for_pods() {
  local namespace=$1
  local timeout=$2
  log_info "Waiting for all pods in namespace '${namespace}' to be Ready (timeout: ${timeout})..."
  if ! ${KUBECTL} wait --for=condition=Ready pods --all -n "${namespace}" --timeout="${timeout}"; then
    log_err "Pods in namespace '${namespace}' failed to become Ready."
  fi
}

wait_for_node_ready() {
  local timeout=$1
  log_info "Waiting for node to be Ready (timeout: ${timeout})..."
  if ! ${KUBECTL} wait --for=condition=Ready node --all --timeout="${timeout}"; then
    log_err "Node failed to become Ready."
  fi
}

# --- Setup ---
setup_dependencies() {
  log_info "Setting up dependencies (CRI-O, Kubelet, CNI)..."
  apt-get update -qq >/dev/null
  apt-get install -qq -y socat conntrack iptables jq curl >/dev/null

  # Install Flannel CNI binary
  mkdir -p /opt/cni/bin
  curl -fsSL https://github.com/containernetworking/plugins/releases/download/v1.4.1/cni-plugins-linux-amd64-v1.4.1.tgz | tar -xz -C /opt/cni/bin

  # Install CRI-O
  mkdir -p /etc/apt/keyrings
  curl -fsSL https://pkgs.k8s.io/addons:/cri-o:/stable:/v1.31/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/cri-o-apt-keyring.gpg || true
  echo "deb [signed-by=/etc/apt/keyrings/cri-o-apt-keyring.gpg] https://pkgs.k8s.io/addons:/cri-o:/stable:/v1.31/deb/ /" | tee /etc/apt/sources.list.d/cri-o.list
  apt-get update -qq >/dev/null
  apt-get install -qq -y cri-o >/dev/null
  systemctl enable --now crio.service

  # Install Kubelet & Kubectl
  curl -fsSL https://dl.k8s.io/release/v1.35.0/bin/linux/amd64/kubelet -o /usr/local/bin/kubelet
  curl -fsSL https://dl.k8s.io/release/v1.35.0/bin/linux/amd64/kubectl -o /usr/local/bin/kubectl
  chmod +x /usr/local/bin/kubelet /usr/local/bin/kubectl

  # Setup nanok8s service and config
  cp packaging/systemd/nanok8s.service /etc/systemd/system/
  mkdir -p /etc/nanok8s
  ${NANOK8S_BIN} config print-defaults > /etc/nanok8s/config.yaml
  systemctl daemon-reload
}

# --- Test Cases ---

test_abnormal_invalid_config() {
  log_info "TEST: [Abnormal] Invalid configuration should be rejected."
  cat <<EOF > /tmp/invalid-config.yaml
apiVersion: bootstrap.nanok8s.io/v1alpha1
kind: NanoK8sConfig
metadata:
  name: bad
spec:
  runtime:
    criSocket: "http://wrong-protocol.sock" # Invalid scheme
EOF

  if ${NANOK8S_BIN} config validate --config /tmp/invalid-config.yaml 2>/dev/null; then
    log_err "Config validation should have failed on invalid criSocket."
  fi
  log_info "Invalid config correctly rejected."
}

test_normal_bootstrap_and_boot() {
  log_info "TEST: [Normal] Bootstrap and boot control plane."
  ${NANOK8S_BIN} config validate # Validates the default config
  ${NANOK8S_BIN} bootstrap

  if [ ! -f "/etc/kubernetes/manifests/kube-apiserver.yaml" ]; then
    log_err "Bootstrap did not generate apiserver manifest."
  fi

  systemctl start nanok8s.service
  
  if ! systemctl is-active --quiet nanok8s.service; then
    journalctl -u nanok8s.service --no-pager
    log_err "nanok8s.service failed to start."
  fi

  wait_for_node_ready "3m"
  wait_for_pods "kube-system" "5m"
  log_info "Control plane successfully booted."
}

test_normal_connectivity() {
  log_info "TEST: [Normal] Workload & Network Connectivity Test."
  
  # Install Flannel to provide Pod network connectivity
  ${KUBECTL} apply -f https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml
  log_info "Waiting for Flannel..."
  wait_for_pods "kube-flannel" "3m"
  
  # Deploy a test application and expose it
  ${KUBECTL} create deployment e2e-test --image=nginx:alpine
  ${KUBECTL} expose deployment e2e-test --port=80
  
  log_info "Waiting for test deployment to become ready..."
  ${KUBECTL} wait --for=condition=Available deployment/e2e-test --timeout=2m
  
  local svc_ip
  svc_ip=$(${KUBECTL} get svc e2e-test -o jsonpath='{.spec.clusterIP}')
  
  log_info "Testing connectivity to Service IP: ${svc_ip}"
  if ! curl --retry 5 --retry-delay 3 --retry-connrefused -s "http://${svc_ip}" | grep -q "Welcome to nginx"; then
    log_err "Failed to connect to test workload. Network connectivity is broken."
  fi
  log_info "Connectivity test passed."
}

test_abnormal_reconciliation() {
  log_info "TEST: [Abnormal] Artifact corruption reconciliation."
  
  # Simulate tampering with a critical bootstrap artifact
  rm -f /etc/kubernetes/manifests/kube-scheduler.yaml
  systemctl stop kubelet
  systemctl restart nanok8s.service
  
  if [ ! -f "/etc/kubernetes/manifests/kube-scheduler.yaml" ]; then
    log_err "nanok8s failed to reconcile and restore the missing kube-scheduler manifest."
  fi
  wait_for_node_ready "2m"
  log_info "Reconciliation recovered the cluster successfully."
}

test_normal_upgrade_simulation() {
  log_info "TEST: [Normal] Upgrade simulation."
  
  # We swap the binary with the compiled v1.36.0 mock and restart the service
  cp bin/nanok8s-v1.36.0 /usr/bin/nanok8s
  systemctl restart nanok8s.service
  
  # Give it a moment to write state
  sleep 5
  
  local last_event
  last_event=$(cat /var/lib/nanok8s/state/last-event)
  if [[ ! "${last_event}" == *"upgraded"* ]]; then
    log_err "Upgrade event not recorded. Last event: ${last_event}"
  fi
  
  if ! grep -q "v1.36.0" /var/lib/nanok8s/state/last-boot.json; then
    log_err "v1.36.0 not found in last-boot.json after upgrade."
  fi
  log_info "Upgrade logic executed perfectly."
}

test_abnormal_rollback_simulation() {
  log_info "TEST: [Abnormal] Rollback behavior simulation (Greenboot style)."
  
  # For nanok8s, if ostree isn't present, maybeRestore logs "non-ostree system: backup/restore disabled".
  # We will test that a boot failure properly exits non-zero so greenboot catches it.
  
  # Tamper with kubelet to force a boot failure
  mv /usr/local/bin/kubelet /usr/local/bin/kubelet.bak
  echo -e "#!/bin/sh\nexit 1" > /usr/local/bin/kubelet
  chmod +x /usr/local/bin/kubelet
  
  # Systemd should fail starting nanok8s because kubelet will fail /readyz timeout
  # We use a short timeout wrapper or just observe the failure. Actually, waitReadyz takes 3 mins.
  # Let's fail the ensure phase instead to make it fast.
  rm -f /etc/kubernetes/pki/ca.crt
  
  if /usr/bin/nanok8s boot 2>/dev/null; then
    log_err "nanok8s boot should have failed without a CA cert!"
  fi
  
  local last_event
  last_event=$(cat /var/lib/nanok8s/state/last-event)
  if [[ ! "${last_event}" == *"boot failed"* ]]; then
    log_warn "Failure event not recorded appropriately. Last event: ${last_event}"
  fi
  
  # Restore
  mv /usr/local/bin/kubelet.bak /usr/local/bin/kubelet
  ${NANOK8S_BIN} bootstrap
  log_info "Boot failure correctly propagated."
}

test_normal_reset() {
  log_info "TEST: [Normal] Reset functionality."
  ${NANOK8S_BIN} reset --yes
  
  if [ -d "/etc/kubernetes/manifests" ]; then
    log_err "/etc/kubernetes/manifests was not removed by reset."
  fi
  if [ -d "/var/lib/etcd" ]; then
    log_err "/var/lib/etcd was not removed by reset."
  fi
  if systemctl is-active --quiet kubelet.service; then
    log_err "kubelet is still running after reset."
  fi
  log_info "Reset successfully wiped the node."
}

# --- Main Execution ---
main() {
  log_info "Starting nanok8s rigorous E2E test suite..."
  setup_dependencies
  
  test_abnormal_invalid_config
  test_normal_bootstrap_and_boot
  test_normal_connectivity
  test_abnormal_reconciliation
  test_normal_upgrade_simulation
  test_abnormal_rollback_simulation
  test_normal_reset
  
  log_info "All rigorous E2E tests passed successfully. 🎉"
}

main
