package main

import (
	"runtime"

	"github.com/spf13/cobra"
)

var (
	AppVersion = "dev"
)

func NewVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Check the version info",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Printf("teldrive %s\n", AppVersion)
			cmd.Printf("- os/type: %s\n", runtime.GOOS)
			cmd.Printf("- os/arch: %s\n", runtime.GOARCH)
			cmd.Printf("- go/version: %s\n", runtime.Version())
			return nil
		},
	}
}
