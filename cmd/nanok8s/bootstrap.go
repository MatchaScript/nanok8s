package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MatchaScript/nanok8s/internal/config"
	"github.com/MatchaScript/nanok8s/internal/kubeadm"
)

func newBootstrapCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "bootstrap",
		Short: "Idempotently generate PKI, kubeconfigs, static pod manifests, and kubelet config",
		Long: "Run on every boot as a oneshot systemd unit. Creates missing state on a " +
			"fresh node and reuses existing state on reboot or after a bootc image update.",
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			fmt.Fprintf(cmd.OutOrStdout(), "bootstrap complete (node=%s, kubernetesVersion=%s)\n",
				nodeName, cfg.Spec.KubernetesVersion)
			return nil
		},
	}
}

// defaultNodeName matches kubeadm/kubelet: lowercased OS hostname.
func defaultNodeName() (string, error) {
	h, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("get hostname: %w", err)
	}
	return strings.ToLower(h), nil
}
