package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/MatchaScript/nanok8s/internal/config"
	"github.com/MatchaScript/nanok8s/internal/kubeadm"
	"github.com/MatchaScript/nanok8s/internal/kubeclient"
	"github.com/MatchaScript/nanok8s/internal/paths"
)

// applyAddonWait bounds how long `apply` blocks on an unresponsive apiserver
// during best-effort addon reconciliation. Kept short so the daily timer
// does not stall when control-plane pods are restarting.
const applyAddonWait = 30 * time.Second

func newApplyCmd(g *globalOpts) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Reconcile on-disk state with the current config (manual or daily timer)",
		Long: "Runs every bootstrap phase, then best-effort applies CoreDNS and " +
			"kube-proxy. When the apiserver is not yet reachable (e.g. cold boot " +
			"before kubelet has started the control-plane pods) the addon step " +
			"is skipped with a warning and retried on the next timer tick.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dryRun {
				return errors.New("--dry-run is not yet implemented")
			}
			cfg, err := config.Load(g.configPath)
			if err != nil {
				return err
			}
			nodeName, err := defaultNodeName()
			if err != nil {
				return err
			}
			if err := kubeadm.Ensure(cfg, kubeadm.DefaultLayout(), nodeName); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "bootstrap phases reconciled")

			// Best-effort addon reconciliation. Short timeout keeps the timer
			// run bounded when the apiserver is unavailable.
			client, err := kubeclient.LoadAdmin(filepath.Join(paths.KubeconfigDir, "admin.conf"))
			if err != nil {
				fmt.Fprintf(cmd.OutOrStderr(), "skip addons: %v\n", err)
				return nil
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), applyAddonWait)
			defer cancel()
			if err := kubeclient.WaitForAPIServer(ctx, client); err != nil {
				fmt.Fprintf(cmd.OutOrStderr(), "skip addons (apiserver not ready): %v\n", err)
				return nil
			}
			if err := kubeadm.EnsureAddons(cfg, kubeadm.DefaultLayout(), client, cmd.OutOrStderr()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "addons reconciled")
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without writing")
	return cmd
}
