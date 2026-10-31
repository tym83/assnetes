package obrabotka

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func obj(kind, ns, name string, replicas int64) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": kind,
		"metadata": map[string]any{"name": name, "namespace": ns},
		"spec":     map[string]any{"replicas": replicas},
		"status":   map[string]any{"replicas": int64(1)},
	}}
	return u
}

// The same books give the same выписка, byte for byte, whatever order the
// documents came in: otherwise every сверка would be a new revision.
func TestPackIsReproducible(t *testing.T) {
	a, err := Pack([]*unstructured.Unstructured{obj("Deployment", "romashka", "vitrina", 5), obj("Deployment", "romashka", "sklad", 1)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Pack([]*unstructured.Unstructured{obj("Deployment", "romashka", "sklad", 1), obj("Deployment", "romashka", "vitrina", 5)})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("the same documents packed differently")
	}
	gz, err := gzip.NewReader(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(tr)
		if bytes.Contains(data, []byte("status")) {
			t.Errorf("%s: a выписка carries no status", h.Name)
		}
		names = append(names, h.Name)
	}
	if len(names) != 2 || names[0] != "deployment-romashka-sklad.yaml" || names[1] != "deployment-romashka-vitrina.yaml" {
		t.Errorf("files: %v", names)
	}
	if _, err := Pack([]*unstructured.Unstructured{obj("Deployment", "romashka", "vitrina", 5), obj("Deployment", "romashka", "vitrina", 3)}); err == nil {
		t.Error("one document twice in a выписка")
	}
}
