package zapros

import (
	"strings"
	"testing"

	"github.com/tym83/assnetes/pkg/bsl"
)

type fake map[string][]string

func (f fake) Table(kind string) ([]bsl.Value, error) {
	var out []bsl.Value
	for _, js := range f[kind] {
		v, err := bsl.ParseJSON([]byte(js), true)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func pod(ns, name, phase, node, cpu string, restarts int) string {
	return `{"Метаданные":{"Имя":"` + name + `","Подразделение":"` + ns + `"},
		"Спецификация":{"ИмяУзла":"` + node + `","Контейнеры":[{"Имя":"c","Ресурсы":{"Запросы":{"Процессор":"` + cpu + `","Память":"1Gi"}}}]},
		"Состояние":{"Фаза":"` + phase + `","СостоянияКонтейнеров":[{"Имя":"c","ЧислоПерезапусков":` + bsl.NewInt(int64(restarts)).Plain() + `}]}}`
}

var data = fake{
	"Поды": {
		pod("бух", "бух-1", "Running", "склад-1", "500m", 0),
		pod("бух", "бух-2", "Running", "склад-2", "250m", 3),
		pod("склад", "склад-1", "Pending", "склад-1", "1", 0),
		pod("склад", "старый", "Succeeded", "склад-2", "2", 0),
	},
	"Узлы": {
		`{"Метаданные":{"Имя":"склад-1"},"Состояние":{"Доступно":{"Процессор":"4","Память":"16Gi","pods":"110"}}}`,
		`{"Метаданные":{"Имя":"склад-2"},"Состояние":{"Доступно":{"Процессор":"2","Память":"8Gi","pods":"110"}}}`,
	},
}

func run(t *testing.T, text string, params map[string]bsl.Value) *bsl.ValueTable {
	t.Helper()
	q, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Execute(q, data, params)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func dump(t *bsl.ValueTable) string {
	var b strings.Builder
	for _, r := range t.Rows() {
		for i, c := range t.Columns() {
			if i > 0 {
				b.WriteString(" ")
			}
			b.WriteString(bsl.ToString(r.Value(c)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestGroupAndOrder(t *testing.T) {
	r := run(t, `
		ВЫБРАТЬ
			Поды.Подразделение КАК Подразделение,
			КОЛИЧЕСТВО(*) КАК Всего,
			СУММА(ВЫБОР КОГДА Поды.Фаза = "Running" ТОГДА 1 ИНАЧЕ 0 КОНЕЦ) КАК Работает
		ИЗ
			Кластер.Поды КАК Поды
		ГДЕ
			Поды.Фаза <> "Succeeded"
		СГРУППИРОВАТЬ ПО
			Поды.Подразделение
		УПОРЯДОЧИТЬ ПО
			Всего УБЫВ`, nil)
	if got, want := dump(r), "бух 2 2\nсклад 1 0\n"; got != want {
		t.Errorf("got\n%swant\n%s", got, want)
	}
}

func TestLikeInParam(t *testing.T) {
	r := run(t, `ВЫБРАТЬ П.Имя ИЗ Кластер.Поды КАК П ГДЕ П.Имя ПОДОБНО "бух%" И П.Подразделение В (&Подразделения) УПОРЯДОЧИТЬ ПО П.Имя`,
		map[string]bsl.Value{"Подразделения": bsl.NewArray("бух")})
	if got := dump(r); got != "бух-1\nбух-2\n" {
		t.Errorf("got %q", got)
	}
}

func TestRegisters(t *testing.T) {
	r := run(t, `ВЫБРАТЬ О.Узел, О.ПроцессорОстаток, О.ПамятьРасход ИЗ РегистрНакопления.РесурсыУзлов.Остатки КАК О УПОРЯДОЧИТЬ ПО О.Узел`, nil)
	if got := dump(r); got != "склад-1 2,5 2\nсклад-2 1,75 1\n" {
		t.Errorf("got %q", got)
	}
	r = run(t, `ВЫБРАТЬ ПЕРВЫЕ 1 О.Под, О.ПерезапускиОборот ИЗ РегистрНакопления.Перезапуски.Обороты КАК О УПОРЯДОЧИТЬ ПО О.ПерезапускиОборот УБЫВ`, nil)
	if got := dump(r); got != "бух-2 3\n" {
		t.Errorf("got %q", got)
	}
}

func TestQueryObject(t *testing.T) {
	it := bsl.New()
	Register(it, data)
	var said []string
	it.Message = func(s string) { said = append(said, s) }
	_, err := it.Run("т", `
		Запрос = Новый Запрос("ВЫБРАТЬ П.Имя КАК Имя ИЗ Кластер.Поды КАК П ГДЕ П.Фаза = &Фаза");
		Запрос.УстановитьПараметр("Фаза", "Pending");
		Выборка = Запрос.Выполнить().Выбрать();
		Пока Выборка.Следующий() Цикл
			Сообщить(Выборка.Имя);
		КонецЦикла;`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(said, ",") != "склад-1" {
		t.Errorf("said %v", said)
	}
}

func TestErrors(t *testing.T) {
	if _, err := Parse("ВЫБРАТЬ ИЗ"); err == nil || !strings.Contains(err.Error(), "Ошибка в тексте запроса") {
		t.Errorf("got %v", err)
	}
}
