package metadannye

import (
	"strings"
	"testing"
)

func TestDefaultsTakeTheTypeOfTheField(t *testing.T) {
	d, err := Parse([]byte("Вид: Т\nВидAPI: T\nМножественное: ts\nРеквизиты:\n  Процессор: {Тип: Строка, ПоУмолчанию: 1}\n  Реплики: {Тип: Целое, ПоУмолчанию: 2}\n"))
	if err != nil {
		t.Fatal(err)
	}
	crd, err := d.CRD()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(crd), `"default": "1"`) || !strings.Contains(string(crd), `"default": 2`) {
		t.Errorf("got %s", crd)
	}
}
