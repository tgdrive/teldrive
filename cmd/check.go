package cmd

import (
	"fmt"
	"reflect"

	"github.com/spf13/cobra"
	"github.com/tgdrive/teldrive/internal/config"
)

func NewCheckCmd() *cobra.Command {
	var cfg config.CheckCmdConfig
	loader := config.NewConfigLoader()

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check command (migrating)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("check command is temporarily disabled during Jet/pgx migration")
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := loader.Load(cmd, &cfg); err != nil {
				return err
			}
			return nil
		},
	}

	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[config.CheckCmdConfig]())
	return cmd
}
