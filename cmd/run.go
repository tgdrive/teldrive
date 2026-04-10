package cmd

import (
	"reflect"

	"github.com/spf13/cobra"
	bootstrap "github.com/tgdrive/teldrive/internal/app"
	"github.com/tgdrive/teldrive/internal/config"
)

func NewRun() *cobra.Command {
	var cfg config.ServerCmdConfig
	loader := config.NewConfigLoader()
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start Teldrive Server",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := bootstrap.New(cmd.Context(), &cfg)
			if err != nil {
				return err
			}
			return app.Run(cmd.Context())
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := loader.Load(cmd, &cfg); err != nil {
				return err
			}
			if err := loader.Validate(&cfg); err != nil {
				return err
			}
			return nil
		},
	}
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[config.ServerCmdConfig]())
	return cmd
}
