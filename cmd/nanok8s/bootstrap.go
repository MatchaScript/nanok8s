package main

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/MatchaScript/nanok8s/internal/config"
)

func newBootstrapCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "bootstrap",
		Short: "Idempotently generate PKI, kubeconfigs, static pod manifests, and kubelet config",
		Long: "Run on every boot as a oneshot systemd unit. Creates missing state on a " +
			"fresh node and reuses existing state on reboot or after a bootc image update.",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(g.configPath)
			if err != nil {
				return err
			}
			_ = cfg
			return errors.New("bootstrap: not implemented yet")
		},
	}
}
