package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode"
)

// Column titles and values of kubepkg's tables, the way the rest of 1сctl
// prints them.
var (
	paketyColumns = map[string]string{
		"NAME": "НАИМЕНОВАНИЕ", "VERSION": "ВЕРСИЯ", "REVISION": "РЕДАКЦИЯ", "READY": "ГОТОВ",
		"REASON": "СОСТОЯНИЕ", "LATEST": "ПОСЛЕДНЯЯ", "REPOSITORY": "ПОСТАВЩИК", "DESCRIPTION": "ОПИСАНИЕ",
		"PRIORITY": "ПРИОРИТЕТ", "PACKAGES": "ПОЗИЦИЙ", "URL": "АДРЕС",
	}
	paketyValues = map[string]string{
		"True": "да", "False": "нет", "Unknown": "неизвестно",
		"ReconciliationSucceeded": "✓ проведён", "Progressing": "○ проводится",
		"VersionNotAvailable": "✗ нет в прайсе", "ReconciliationFailed": "✗ не проведён",
		"RequirementsNotMet": "○ ждёт поставки", "ConflictingRequirements": "✗ требования не сходятся",
		"VersionMismatch": "✗ не та версия", "VariantNotFound": "✗ нет такой комплектации",
		"PackageSourceNotFound": "✗ нет поставщика", "InvalidConfiguration": "✗ ошибка в заявке",
		"CRDOwnershipConflict": "✗ расширение конфигурации чужое", "Conflict": "✗ конфликт",
		"UpgradeFailed": "✗ обновление не прошло", "UpgradeRolledBack": "↩ обновление сторнировано",
		"RolledBack": "↩ сторнировано", "Deleting": "✗ списывается",
	}
)

// translateTable rewrites a table of kubepkg, aligned by tabwriter, with
// Russian titles and values. Columns are cut where the titles start, so
// empty cells keep their place.
func translateTable(out io.Writer, table string) error {
	lines := strings.Split(strings.TrimRight(table, "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}
	header := []rune(lines[0])
	var starts []int
	for i, r := range header {
		if !unicode.IsSpace(r) && (i == 0 || unicode.IsSpace(header[i-1])) {
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 || !strings.HasPrefix(lines[0], "NAME") {
		_, err := fmt.Fprintln(out, table)
		return err
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for n, line := range lines {
		rs := []rune(line)
		cells := make([]string, len(starts))
		for i, s := range starts {
			end := len(rs)
			if i+1 < len(starts) && starts[i+1] < end {
				end = starts[i+1]
			}
			if s < end {
				cells[i] = strings.TrimSpace(string(rs[s:end]))
			}
			dict := paketyValues
			if n == 0 {
				dict = paketyColumns
			}
			if ru, ok := dict[cells[i]]; ok {
				cells[i] = ru
			}
		}
		fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
	return w.Flush()
}
