package zapros

import (
	"testing"
)

func operation(name, date, entries, extra string, taken bool) string {
	t := "false"
	if taken {
		t = "true"
	}
	return `{"Метаданные":{"Имя":"` + name + `","Подразделение":"бух"},
		"Спецификация":{"Книга":"Основная","Дата":"` + date + `","Проводки":[` + entries + `]` + extra + `},
		"Состояние":{"Учтена":` + t + `}}`
}

var books = fake{
	"ГлавныеКниги": {`{"Метаданные":{"Имя":"Основная","Подразделение":"бух"},
		"Спецификация":{"ДатаЗапрета":"2026-09-30","ПланСчетов":[{"Код":"01","Наименование":"Основные средства"},{"Код":"80","Наименование":"Уставный капитал","Вид":"Пассивный"}]}}`},
	"Операции": {
		operation("БП-1", "2026-09-15", `{"Дт":"01/Витрина","Кт":"80","Сумма":2}`, "", true),
		operation("БП-2", "2026-10-20", `{"Дт":"01/Витрина","Кт":"80","Сумма":3}`, "", true),
		operation("БП-3", "2026-10-21", `{"Дт":"01/Склад","Кт":"80","Сумма":4}`, "", true),
		operation("Сторно БП-3", "2026-10-22", `{"Дт":"01/Склад","Кт":"80","Сумма":-4}`, `,"Сторнирует":"БП-3"`, true),
		operation("БП-5", "2026-10-23", `{"Дт":"01/Витрина","Кт":"80","Сумма":100}`, "", false),
	},
}

// The turnover balance sheet of the open period: what came before the
// closed date is the opening balance, a сторно takes its entries back out
// of the turnover, and what the books did not take is not there.
func TestBalancesAndTurnovers(t *testing.T) {
	q, err := Parse(`ВЫБРАТЬ О.Счёт, О.СальдоНачальноеДт, О.СальдоНачальноеКт, О.ОборотДт, О.ОборотКт, О.СальдоКонечноеДт, О.СальдоКонечноеКт
		ИЗ РегистрБухгалтерии.Хозрасчётный.ОстаткиИОбороты КАК О УПОРЯДОЧИТЬ ПО О.Счёт`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Execute(q, books, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "01 2 0 3 0 5 0\n" +
		"01/Витрина 2 0 3 0 5 0\n" +
		"01/Склад 0 0 0 0 0 0\n" +
		"80 0 2 0 3 0 5\n"
	if got := dump(r); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
