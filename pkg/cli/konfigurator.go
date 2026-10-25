package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/metadannye"
)

func newKonfiguratorCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:     "конфигуратор КАТАЛОГ",
		Aliases: []string{"designer"},
		Short:   "выгрузить виды обработки из её Метаданные*.yaml в CRD",
		Long: "Читает описания видов обработки (реквизиты с типами, как в метаданных 1С) и печатает\n" +
			"CustomResourceDefinition, которые принимает API: так виды ставятся пакетом.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			files, _ := filepath.Glob(filepath.Join(args[0], "Метаданные*.yaml"))
			more, _ := filepath.Glob(filepath.Join(args[0], "metadannye*.yaml"))
			files = append(files, more...)
			sort.Strings(files)
			if len(files) == 0 {
				return fmt.Errorf("в каталоге %s нет описаний видов (Метаданные*.yaml)", args[0])
			}
			var docs []string
			for _, f := range files {
				data, err := os.ReadFile(f)
				if err != nil {
					return err
				}
				d, err := metadannye.Parse(data)
				if err != nil {
					return fmt.Errorf("%s: %w", f, err)
				}
				js, err := d.CRD()
				if err != nil {
					return err
				}
				// as it is: the реквизиты are Cyrillic in the API, not to be translated
				crd, err := bsl.ParseJSON(js, true)
				if err != nil {
					return err
				}
				y, err := toYAML(crd)
				if err != nil {
					return err
				}
				docs = append(docs, y)
			}
			fmt.Fprint(o.out, strings.Join(docs, "---\n"))
			return nil
		},
	}
}
