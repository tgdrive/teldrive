package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tgdrive/teldrive/internal/version"
)

func NewVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Check the version info",
		RunE: func(cmd *cobra.Command, args []string) error {
			v := version.GetVersionInfo()
			cmd.Printf("teldrive %s\n", v.Version)
			cmd.Printf("- commit: %s\n", v.CommitSHA)
			cmd.Printf("- os/type: %s\n", v.Os)
			cmd.Printf("- os/arch: %s\n", v.Arch)
			cmd.Printf("- go/version: %s\n", v.GoVersion)
			return nil
		},
	}
}
