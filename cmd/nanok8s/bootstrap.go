package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MatchaScript/nanok8s/internal/config"
	"github.com/MatchaScript/nanok8s/internal/kubeadm"
	"github.com/MatchaScript/nanok8s/internal/state"
	"github.com/MatchaScript/nanok8s/internal/version"
)

// newBootstrapCmd is the one-time initialisation verb operators run on a
// fresh node to write PKI, kubeconfigs, static pod manifests, and the
// kubelet config. Subsequent boots are handled automatically by
// nanok8s.service (see `boot` subcommand). Refuses to run on a node that
// already has nanok8s state; `--force` exists to re-init after manual
// recovery.
func newBootstrapCmd(g *globalOpts) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "bootstrap",
		Aliases: []string{"init"},
		Short:   "Initialise a fresh node (run once per install)",
		Long: "Writes PKI, kubeconfigs, static pod manifests, and kubelet config. " +
			"After this completes, enable nanok8s.service so subsequent boots " +
			"reconcile automatically. Refuses to run if nanok8s state already " +
			"exists on this node; pass --force to overwrite.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			existed, err := state.Exists()
			if err != nil {
				return err
			}
			if existed && !force {
				return errors.New("nanok8s state already exists; re-run with --force to re-initialise, " +
					"or use `nanok8s reset --yes` first")
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
			// Record a marker so state.Exists() reports true immediately,
			// before any nanok8s.service boot has completed. Without this
			// a second `nanok8s bootstrap` without --force would pass the
			// refusal check even though the node is already initialised.
			if err := state.WriteLastEvent(fmt.Sprintf("bootstrapped at %s", version.KubernetesVersion)); err != nil {
				return fmt.Errorf("record bootstrap event: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"bootstrap complete (node=%s, kubernetesVersion=%s)\n"+
					"next step: `systemctl enable --now nanok8s.service`\n",
				nodeName, version.KubernetesVersion)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing nanok8s state")
	return cmd
}

// defaultNodeName matches kubeadm/kubelet: lowercased OS hostname.
func defaultNodeName() (string, error) {
	h, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("get hostname: %w", err)
	}
	return strings.ToLower(h), nil
}
