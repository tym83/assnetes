package obrabotka

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tym83/assnetes/pkg/metadannye"
)

// The operators shipped with the distribution must compile and declare
// what they handle.
func TestShippedObrabotki(t *testing.T) {
	dirs, err := filepath.Glob("../../obrabotki/*")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no обработки: %v", err)
	}
	for _, dir := range dirs {
		ob, err := Load(dir)
		if err != nil {
			t.Errorf("%s: %v", dir, err)
			continue
		}
		for _, m := range ob.Checks {
			if _, err := ob.webhook(m); err != nil {
				t.Errorf("%s: %v", dir, err)
			}
		}
		for _, f := range ob.CRDs {
			data, _ := os.ReadFile(f)
			d, err := metadannye.Parse(data)
			if err != nil {
				t.Errorf("%s: %v", f, err)
				continue
			}
			if _, err := d.CRD(); err != nil {
				t.Errorf("%s: %v", f, err)
			}
		}
	}
}
