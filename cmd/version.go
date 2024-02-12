package cmd

import (
	"runtime"

	"github.com/divyam234/teldrive/internal/config"
	"github.com/spf13/cobra"
)

func NewVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Check the version info",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Printf("teldrive %s\n", config.Version)
			cmd.Printf("- os/type: %s\n", runtime.GOOS)
			cmd.Printf("- os/arch: %s\n", runtime.GOARCH)
			cmd.Printf("- go/version: %s\n", runtime.Version())
			return nil
		},
	}
}
