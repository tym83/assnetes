package obrabotka

import (
	"os"
	"path/filepath"
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"

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

func TestEverythingChecksPeopleOnly(t *testing.T) {
	rules, conds := everything([]admissionregistrationv1.OperationType{
		admissionregistrationv1.Create, admissionregistrationv1.Delete, admissionregistrationv1.Connect,
	})
	if len(rules) != 2 || rules[0].Resources[0] != "*" || len(rules[0].Operations) != 2 {
		t.Fatalf("changes of every kind expected first: %+v", rules)
	}
	if rules[1].Operations[0] != admissionregistrationv1.Connect || rules[1].Resources[0] != "pods/exec" {
		t.Fatalf("connections to pods expected second: %+v", rules[1])
	}
	if len(conds) != 1 || conds[0].Expression != `!request.userInfo.username.startsWith("system:")` {
		t.Fatalf("Kubernetes' own users must be left out: %+v", conds)
	}
}
