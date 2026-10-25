package manifest

import (
	"context"
	"strings"
	"testing"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/perevod"
)

const deployment = `
#Область Бухгалтерия

Развёртывание = Новый Развёртывание("Бухгалтерия");
Развёртывание.Метаданные.Метки.Вставить("приложение", "бухгалтерия");
Развёртывание.Спецификация.Реплики = Число(Параметры.Реплики);
Развёртывание.Спецификация.Отбор.ПоМеткам.Вставить("приложение", "бухгалтерия");
Развёртывание.Спецификация.Шаблон.Метаданные.Метки.Вставить("приложение", "бухгалтерия");

Контейнер = Развёртывание.Спецификация.Шаблон.Спецификация.Контейнеры.Добавить();
Контейнер.Имя = "веб";
Контейнер.Образ = "nginx:1.29";
Контейнер.Ресурсы.Запросы.Процессор = "100m";
Порт = Контейнер.Порты.Добавить();
Порт.ПортКонтейнера = 80;

Если Развёртывание.Спецификация.Реплики > 1 Тогда
	Сообщить("Две реплики? Кто это согласовывал?");
КонецЕсли;

Провести(Развёртывание);

#КонецОбласти
`

func TestRunDeployment(t *testing.T) {
	var said []string
	res, err := Run(context.Background(), "бухгалтерия.bsl", deployment, Options{
		Params:    map[string]string{"Реплики": "2"},
		Namespace: "Ромашка",
		Message:   func(s string) { said = append(said, s) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Objects) != 1 {
		t.Fatalf("posted %d objects", len(res.Objects))
	}
	js, _ := bsl.WriteJSON(res.Objects[0], "")
	for _, want := range []string{`"kind":"Deployment"`, `"name":"buhgalteriya"`, `"namespace":"romashka"`,
		`"replicas":2`, `"containerPort":80`, `"cpu":"100m"`, `"matchLabels":{"prilozhenie":"buhgalteriya"}`} {
		if !strings.Contains(js, want) {
			t.Errorf("lacks %s:\n%s", want, js)
		}
	}
	if strings.Contains(js, "status") || strings.Contains(js, "{}") {
		t.Errorf("empty fields leak:\n%s", js)
	}
	if len(said) != 1 {
		t.Errorf("said %v", said)
	}
}

func TestExportRoundTrip(t *testing.T) {
	res, err := Run(context.Background(), "м.bsl", deployment, Options{Params: map[string]string{"Реплики": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	ru, err := perevod.FromAPI(res.Objects[0])
	if err != nil {
		t.Fatal(err)
	}
	src, err := Export(ru)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Run(context.Background(), "выгрузка.bsl", src, Options{})
	if err != nil {
		t.Fatalf("%v\n%s", err, src)
	}
	a, _ := bsl.WriteJSON(res.Objects[0], "")
	b, _ := bsl.WriteJSON(again.Objects[0], "")
	if a != b {
		t.Errorf("round trip differs:\n%s\n%s\n%s", a, b, src)
	}
}

func TestNothingPosted(t *testing.T) {
	_, err := Run(context.Background(), "пусто.bsl", `А = 1;`, Options{})
	if err == nil || !strings.Contains(err.Error(), "Провести") {
		t.Fatalf("got %v", err)
	}
}
