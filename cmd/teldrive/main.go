package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var configFile string

const versionString = "1.1.0"

var rootCmd = &cobra.Command{
	Use:               "teldrive [command]",
	Short:             "Teledrive",
	Example:           "teldrive run",
	Version:           versionString,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.PersistentFlags().StringVarP(&configFile, "config", "", "", "config file path")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Printf("failed to execute command. err: %v", err)
		os.Exit(1)
	}
}
