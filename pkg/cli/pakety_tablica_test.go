package cli

import (
	"strings"
	"testing"
)

func TestTranslateTableKeepsEmptyCells(t *testing.T) {
	in := "NAME                VERSION     REVISION  READY  REASON\n" +
		"1c-bitrix           0.2.0-rc.2  3         True   ReconciliationSucceeded\n" +
		"assnetes                        0         False  VersionNotAvailable\n"
	var out strings.Builder
	if err := translateTable(&out, in); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if !strings.HasPrefix(lines[0], "НАИМЕНОВАНИЕ") || !strings.Contains(lines[0], "СОСТОЯНИЕ") {
		t.Errorf("titles not translated: %q", lines[0])
	}
	if f := strings.Fields(lines[2]); len(f) < 4 || f[1] != "0" || f[2] != "нет" {
		t.Errorf("empty version moved the other cells: %q", lines[2])
	}
	if !strings.Contains(out.String(), "✓ проведён") {
		t.Errorf("reason not translated:\n%s", out.String())
	}
}
