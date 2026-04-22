package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func newStatusCmd(_ *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report health of control plane components, kubelet, and certificates",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("status: not implemented yet")
		},
	}
}
