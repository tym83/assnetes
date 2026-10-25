package perevod

import (
	"strings"
	"testing"

	"github.com/tym83/assnetes/pkg/bsl"
)

func TestDictionaryIsUnambiguous(t *testing.T) {
	seen := map[string]string{}
	for _, p := range fieldPairs {
		if prev, ok := seen[fold(p[1])]; ok && prev != p[0] {
			t.Errorf("«%s» stands for both %s and %s", p[1], prev, p[0])
		}
		seen[fold(p[1])] = p[0]
	}
	names := map[string]string{}
	for _, k := range Kinds {
		for _, n := range append([]string{k.Ru, k.Plural}, k.Aliases...) {
			if prev, ok := names[fold(n)]; ok && prev != k.Kind {
				t.Errorf("«%s» names both %s and %s", n, prev, k.Kind)
			}
			names[fold(n)] = k.Kind
		}
	}
}

func TestTranslit(t *testing.T) {
	for in, want := range map[string]string{
		"бухгалтерия":     "buhgalteriya",
		"Тамара Петровна": "tamara-petrovna",
		"склад-1":         "sklad-1",
		"Объём":           "obyom",
		"web":             "web",
		"!!!":             "bez-imeni",
		"Счёт 51 (рубли)": "schyot-51-rubli",
	} {
		if got := Translit(in); got != want {
			t.Errorf("Translit(%q) = %q, want %q", in, got, want)
		}
	}
}

func parse(t *testing.T, js string) bsl.Value {
	t.Helper()
	v, err := bsl.ParseJSON([]byte(js), true)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRoundTrip(t *testing.T) {
	in := parse(t, `{
		"Вид": "Развёртывание",
		"Метаданные": {"Имя": "Бухгалтерия", "Подразделение": "ромашка",
			"Метки": {"приложение": "Бухгалтерия", "app.kubernetes.io/part-of": "assnetes"}},
		"Спецификация": {"Реплики": 2,
			"Отбор": {"ПоМеткам": {"приложение": "Бухгалтерия"}},
			"Шаблон": {"Метаданные": {"Метки": {"приложение": "Бухгалтерия"}},
				"Спецификация": {"Контейнеры": [{"Имя": "веб", "Образ": "nginx:1.29",
					"Ресурсы": {"Запросы": {"Процессор": "100m", "Память": "128Mi"}}}]}}}}`)
	api, err := ToAPI(in)
	if err != nil {
		t.Fatal(err)
	}
	js, _ := bsl.WriteJSON(api, "")
	for _, want := range []string{
		`"apiVersion":"apps/v1"`, `"kind":"Deployment"`, `"name":"buhgalteriya"`, `"namespace":"romashka"`,
		`"prilozhenie":"buhgalteriya"`, `"app.kubernetes.io/part-of":"assnetes"`, `"replicas":2`,
		`"matchLabels":{"prilozhenie":"buhgalteriya"}`, `"name":"veb"`, `"cpu":"100m"`,
		`"` + AnnName + `":"Бухгалтерия"`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("API form lacks %s:\n%s", want, js)
		}
	}
	back, err := FromAPI(parse(t, js))
	if err != nil {
		t.Fatal(err)
	}
	ru, _ := bsl.WriteJSON(back, "")
	for _, want := range []string{`"Вид":"Развёртывание"`, `"Имя":"Бухгалтерия"`, `"приложение":"Бухгалтерия"`, `"Процессор":"100m"`} {
		if !strings.Contains(ru, want) {
			t.Errorf("Russian form lacks %s:\n%s", want, ru)
		}
	}
	if strings.Contains(ru, "naimenovanie") {
		t.Errorf("service annotations leak into the display:\n%s", ru)
	}
}

func TestNativeKindsKeepCyrillicFields(t *testing.T) {
	in := parse(t, `{"ВерсияAPI": "оркестратор.бух/v1", "Вид": "ИнформационнаяБаза",
		"Метаданные": {"Имя": "бухгалтерия"}, "Спецификация": {"Конфигурация": "БухгалтерияПредприятия", "Пользователей": 10}}`)
	api, err := ToAPI(in)
	if err != nil {
		t.Fatal(err)
	}
	js, _ := bsl.WriteJSON(api, "")
	for _, want := range []string{`"apiVersion":"` + Group + `/v1"`, `"kind":"Infobase"`, `"Конфигурация":"БухгалтерияПредприятия"`} {
		if !strings.Contains(js, want) {
			t.Errorf("lacks %s: %s", want, js)
		}
	}
}

func TestServiceSelectorIsData(t *testing.T) {
	api, err := ToAPI(parse(t, `{"Вид": "Услуга", "Метаданные": {"Имя": "веб"},
		"Спецификация": {"Отбор": {"приложение": "склад"}, "Порты": [{"Порт": 80}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	js, _ := bsl.WriteJSON(api, "")
	if !strings.Contains(js, `"selector":{"prilozhenie":"sklad"}`) || !strings.Contains(js, `"port":80`) {
		t.Errorf("got %s", js)
	}
}

func TestLookupKindInAnyCase(t *testing.T) {
	for in, want := range map[string]string{
		"услугу": "Service", "Услуги": "Service", "развёртыванием": "Deployment", "поды": "Pod",
		"узлы": "Node", "ИнформационнуюБазу": "Infobase", "deployments": "Deployment", "справочников": "ConfigMap",
	} {
		k, ok := LookupKind(in)
		if !ok || k.Kind != want {
			t.Errorf("LookupKind(%q) = %v, want %s", in, k, want)
		}
	}
}
