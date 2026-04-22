package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/MatchaScript/nanok8s/internal/paths"
	"github.com/MatchaScript/nanok8s/internal/version"
)

type globalOpts struct {
	configPath string
}

func newRootCmd() *cobra.Command {
	opts := &globalOpts{}

	cmd := &cobra.Command{
		Use:           "nanok8s",
		Short:         "Minimal single-node Kubernetes for bootc-style edge deployments",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&opts.configPath, "config", paths.ConfigFile, "path to NanoK8sConfig YAML")

	cmd.AddCommand(
		newBootstrapCmd(opts),
		newApplyCmd(opts),
		newStatusCmd(opts),
		newConfigCmd(opts),
		newVersionCmd(),
	)
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build and target versions",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Printf("nanok8s   kubernetes=%s commit=%s built=%s\n",
				version.KubernetesVersion, version.GitCommit, version.BuildDate)
			return nil
		},
	}
}
