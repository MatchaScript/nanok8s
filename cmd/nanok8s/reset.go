package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/MatchaScript/nanok8s/internal/paths"
)

// newResetCmd tears the node back down to the state a fresh bootstrap
// would expect. The implementation mirrors `kubeadm reset --force`:
//
//  1. Stop kubelet (so static pods are not restarted mid-cleanup).
//  2. Stop and remove every container known to CRI-O via `crictl`.
//  3. Remove the managed filesystem paths (/etc/kubernetes, /var/lib/etcd,
//     /var/lib/kubelet, /var/lib/nanok8s).
//  4. Delete CNI virtual interfaces (cni0, flannel.1, kube-ipvs0, …) so
//     the next cluster's CNI starts from a clean slate.
//  5. Flush iptables (filter / nat / mangle chains + user-defined chains)
//     and ipvs rules.
//
// `--yes` is required; this operation is destructive and irreversible.
func newResetCmd(_ *globalOpts) *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Tear down all nanok8s-managed state (matches `kubeadm reset`)",
		Long: "Stops kubelet, removes CRI containers, wipes /etc/kubernetes, " +
			"/var/lib/etcd, /var/lib/kubelet, /var/lib/nanok8s, deletes CNI " +
			"network interfaces, and flushes iptables and ipvs rules. " +
			"Intended for test beds or when re-bootstrapping from scratch.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !confirm {
				return errors.New("refusing to proceed without --yes (this is destructive)")
			}
			return runReset(cmd.Context(), cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&confirm, "yes", false, "confirm the destructive operation")
	return cmd
}

func runReset(ctx context.Context, out io.Writer) error {
	logf := func(format string, a ...any) { fmt.Fprintf(out, "[reset] "+format+"\n", a...) }

	stopKubelet(ctx, logf)
	cleanupCRIContainers(ctx, logf)

	for _, t := range []string{
		paths.KubernetesDir,
		paths.EtcdDataDir,
		paths.KubeletDir,
		paths.NanoK8sVarDir,
	} {
		if err := os.RemoveAll(t); err != nil {
			return fmt.Errorf("remove %s: %w", t, err)
		}
		logf("removed %s", t)
	}

	deleteCNIInterfaces(ctx, logf)
	flushIptables(ctx, logf)
	flushIPVS(ctx, logf)

	return nil
}

// stopKubelet stops kubelet.service so static pods are not brought back
// up mid-cleanup. Non-fatal: on a fresh node kubelet may not be installed
// or enabled.
func stopKubelet(ctx context.Context, logf func(string, ...any)) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "systemctl", "stop", "kubelet.service")
	if out, err := cmd.CombinedOutput(); err != nil {
		logf("systemctl stop kubelet.service (continuing): %v: %s", err, strings.TrimSpace(string(out)))
		return
	}
	logf("stopped kubelet.service")
}

// cleanupCRIContainers asks crictl (wired to the CRI-O socket) to stop
// and remove every container on the node. kubelet owns pod lifecycle in
// normal operation, but after `systemctl stop kubelet` the containers
// linger and hold open files inside /var/lib/kubelet until crictl rm.
func cleanupCRIContainers(ctx context.Context, logf func(string, ...any)) {
	if _, err := exec.LookPath("crictl"); err != nil {
		logf("crictl not found, skipping container cleanup")
		return
	}
	ids, err := crictlListContainers(ctx)
	if err != nil {
		logf("list CRI containers (continuing): %v", err)
		return
	}
	if len(ids) == 0 {
		logf("no CRI containers to remove")
		return
	}
	// stop+rm in batches; crictl accepts multiple ids per invocation.
	if err := crictlRun(ctx, append([]string{"stop", "--timeout", "5"}, ids...)...); err != nil {
		logf("crictl stop (continuing): %v", err)
	}
	if err := crictlRun(ctx, append([]string{"rm", "--force"}, ids...)...); err != nil {
		logf("crictl rm (continuing): %v", err)
	}
	logf("removed %d CRI containers", len(ids))
}

func crictlListContainers(ctx context.Context) ([]string, error) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "crictl", "ps", "--all", "--quiet")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("crictl ps: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var ids []string
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, scanner.Err()
}

func crictlRun(ctx context.Context, args ...string) error {
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "crictl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("crictl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// cniInterfaces enumerates the virtual network interfaces that CNI
// plugins typically create. kubeadm reset hard-codes a similar list.
var cniInterfaces = []string{
	"cni0",
	"flannel.1",
	"cilium_host",
	"cilium_net",
	"cilium_vxlan",
	"kube-ipvs0",
	"dummy0",
	"weave",
	"vxlan.calico",
}

func deleteCNIInterfaces(ctx context.Context, logf func(string, ...any)) {
	if _, err := exec.LookPath("ip"); err != nil {
		logf("iproute2 `ip` not found, skipping CNI interface cleanup")
		return
	}
	for _, name := range cniInterfaces {
		if !interfaceExists(ctx, name) {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		cmd := exec.CommandContext(cctx, "ip", "link", "delete", name)
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			logf("ip link delete %s (continuing): %v: %s", name, err, strings.TrimSpace(string(out)))
			continue
		}
		logf("deleted interface %s", name)
	}
}

func interfaceExists(ctx context.Context, name string) bool {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "ip", "link", "show", name)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

// flushIptables runs iptables -F / -X on the three tables kubeadm/CNI
// plugins touch. Missing iptables binary is not fatal (hosts using pure
// nftables may omit it).
func flushIptables(ctx context.Context, logf func(string, ...any)) {
	if _, err := exec.LookPath("iptables"); err != nil {
		logf("iptables not found, skipping iptables flush")
		return
	}
	tables := []string{"filter", "nat", "mangle"}
	for _, t := range tables {
		for _, op := range [][]string{{"-F"}, {"-X"}} {
			args := append([]string{"-t", t}, op...)
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			cmd := exec.CommandContext(cctx, "iptables", args...)
			out, err := cmd.CombinedOutput()
			cancel()
			if err != nil {
				logf("iptables %s (continuing): %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
			}
		}
	}
	logf("flushed iptables (filter/nat/mangle)")
}

// flushIPVS clears the IPVS table kube-proxy uses in IPVS mode. ipvsadm
// may not be installed on iptables-only clusters; treat as optional.
func flushIPVS(ctx context.Context, logf func(string, ...any)) {
	if _, err := exec.LookPath("ipvsadm"); err != nil {
		logf("ipvsadm not found, skipping IPVS flush")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "ipvsadm", "-C")
	if out, err := cmd.CombinedOutput(); err != nil {
		logf("ipvsadm -C (continuing): %v: %s", err, strings.TrimSpace(string(out)))
		return
	}
	logf("flushed IPVS rules")
}
