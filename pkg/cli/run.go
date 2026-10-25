package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tym83/assnetes/pkg/bsl"
)

func newRunCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:     "выполнить ФАЙЛ.os",
		Aliases: []string{"run"},
		Short:   "выполнить модуль на языке 1С (без доступа к кластеру)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("модуль не найден: %w", err)
			}
			it := bsl.New()
			it.Out = o.out
			it.WithContext(cmd.Context())
			it.Env = os.LookupEnv
			_, err = it.Run(args[0], string(src))
			return err
		},
	}
}
