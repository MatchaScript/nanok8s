package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/MatchaScript/nanok8s/internal/config"
	"github.com/MatchaScript/nanok8s/internal/kubeadm"
)

func newApplyCmd(g *globalOpts) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Reconcile on-disk state with the current config (manual or daily timer)",
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
			fmt.Fprintln(cmd.OutOrStdout(), "apply complete")
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without writing")
	return cmd
}
