package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/MatchaScript/nanok8s/internal/config"
	"github.com/MatchaScript/nanok8s/internal/kubeadm"
	"github.com/MatchaScript/nanok8s/internal/kubeclient"
	"github.com/MatchaScript/nanok8s/internal/paths"
)

func newAddonsCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addons",
		Short: "Manage in-cluster addons (CoreDNS, kube-proxy)",
	}
	cmd.AddCommand(newAddonsApplyCmd(g))
	return cmd
}

func newAddonsApplyCmd(g *globalOpts) *cobra.Command {
	var waitTimeout time.Duration
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply CoreDNS and kube-proxy to a running control plane",
		Long: "Waits for the apiserver to become ready, then applies the CoreDNS and " +
			"kube-proxy manifests. CNI is intentionally out of scope and must be " +
			"installed separately.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(g.configPath)
			if err != nil {
				return err
			}

			kubeconfigPath := filepath.Join(paths.KubeconfigDir, "admin.conf")
			client, err := kubeclient.LoadAdmin(kubeconfigPath)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), waitTimeout)
			defer cancel()
			if err := kubeclient.WaitForAPIServer(ctx, client); err != nil {
				return err
			}

			if err := kubeadm.EnsureAddons(cfg, kubeadm.DefaultLayout(), client, cmd.OutOrStderr()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "addons applied")
			return nil
		},
	}
	cmd.Flags().DurationVar(&waitTimeout, "wait", 2*time.Minute, "how long to wait for apiserver readiness")
	return cmd
}
