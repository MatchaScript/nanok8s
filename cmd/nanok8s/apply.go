package main

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/MatchaScript/nanok8s/internal/config"
)

func newApplyCmd(g *globalOpts) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Reconcile on-disk state with the current config (manual or daily timer)",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(g.configPath)
			if err != nil {
				return err
			}
			_ = cfg
			_ = dryRun
			return errors.New("apply: not implemented yet")
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without writing")
	return cmd
}
