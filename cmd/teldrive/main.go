package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:               "teldrive [command]",
	Short:             "Teldrive",
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	runCmd := NewRun()
	rootCmd.AddCommand(runCmd, NewVersion())

}
func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Printf("failed to execute command. err: %v", err)
		os.Exit(1)
	}
}
