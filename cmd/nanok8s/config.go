package main

import (
	"fmt"

	"github.com/spf13/cobra"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
	"github.com/MatchaScript/nanok8s/internal/config"
)

func newConfigCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and validate NanoK8sConfig",
	}
	cmd.AddCommand(newConfigPrintDefaultsCmd(), newConfigValidateCmd(g))
	return cmd
}

func newConfigPrintDefaultsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "print-defaults",
		Short: "Print a NanoK8sConfig with all defaults applied",
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := config.Marshal(v1alpha1.NewDefault())
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
}

func newConfigValidateCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Load the config file, apply defaults, and validate it",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(g.configPath)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "config %s is valid (kubernetesVersion=%s, advertiseAddress=%s)\n",
				g.configPath, cfg.Spec.KubernetesVersion, cfg.Spec.ControlPlane.AdvertiseAddress)
			return nil
		},
	}
}
