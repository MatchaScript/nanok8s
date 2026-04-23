package main

import (
	"github.com/spf13/cobra"

	"github.com/MatchaScript/nanok8s/internal/config"
	"github.com/MatchaScript/nanok8s/internal/lifecycle"
	"github.com/MatchaScript/nanok8s/internal/version"
)

// newBootCmd returns the hidden subcommand nanok8s.service invokes.
// Operators do not see this in help output; their supported verbs are
// `bootstrap` and `reset`. `boot` runs the oneshot orchestration
// (restore-if-needed -> snapshot -> Ensure -> kubelet -> /readyz ->
// mark valid) and returns non-zero to signal a failed boot to
// greenboot.
func newBootCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:    "boot",
		Short:  "Internal: run the oneshot boot lifecycle (invoked by nanok8s.service)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(g.configPath)
			if err != nil {
				return err
			}
			nodeName, err := defaultNodeName()
			if err != nil {
				return err
			}
			return lifecycle.Boot(cmd.Context(), cfg, nodeName, version.KubernetesVersion, cmd.ErrOrStderr())
		},
	}
}
