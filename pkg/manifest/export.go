package manifest

import (
	"fmt"
	"strings"
	"time"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/perevod"
)

// fields the cluster fills in; a manifest does not write them.
var serverFields = map[string]bool{
	"Состояние": true, "Идентификатор": true, "ВерсияРесурса": true, "Поколение": true,
	"ДатаСоздания": true, "Владельцы": true, "managedFields": true, "selfLink": true,
}

var elementNames = map[string]string{
	"Контейнеры": "Контейнер", "НачальныеКонтейнеры": "Контейнер", "Порты": "Порт",
	"Окружение": "Переменная", "Тома": "Том", "ПодключенияТомов": "Подключение",
	"Правила": "Правило", "Пути": "Путь", "Допуски": "Допуск", "Субъекты": "Субъект",
}

// Export writes an object, in the Russian form FromAPI gives, as the
// source of a manifest that posts it.
func Export(obj *bsl.Map) (string, error) {
	kv, _ := obj.Lookup("Вид")
	kindName := bsl.ToString(kv)
	kind, known := perevod.LookupKind(kindName)
	varName := "Объект"
	ctor := fmt.Sprintf("Новый ОбъектКластера(%s)", quote(kindName))
	if known {
		varName = kind.Ru
		ctor = "Новый " + kind.Ru
	}
	var name, ns string
	if mdv, ok := obj.Lookup("Метаданные"); ok {
		if md, ok := mdv.(*bsl.Map); ok {
			if v, ok := md.Lookup("Имя"); ok {
				name = bsl.ToString(v)
			}
			if v, ok := md.Lookup("Подразделение"); ok {
				ns = bsl.ToString(v)
			}
		}
	}
	args := []string{quote(name)}
	if ns != "" && known && kind.Namespaced {
		args = append(args, quote(ns))
	}
	if known {
		ctor += "(" + strings.Join(args, ", ") + ")"
	}
	e := &exporter{}
	e.line("%s = %s;", varName, ctor)
	for _, k := range obj.Keys() {
		key := bsl.ToString(k)
		v, _ := obj.Lookup(k)
		switch key {
		case "Вид":
			continue
		case "ВерсияAPI":
			if known {
				continue
			}
		case "Метаданные":
			if !known {
				e.line("%s.Метаданные.Имя = %s;", varName, quote(name))
			}
			e.fields(varName+".Метаданные", v, map[string]bool{"Имя": true, "Подразделение": known && kind.Namespaced})
			continue
		}
		if serverFields[key] {
			continue
		}
		e.assign(varName, key, v)
	}
	e.line("")
	e.line("Провести(%s);", varName)
	return e.b.String(), nil
}

type exporter struct{ b strings.Builder }

func (e *exporter) line(format string, args ...any) {
	fmt.Fprintf(&e.b, format, args...)
	e.b.WriteByte('\n')
}

func (e *exporter) fields(target string, v bsl.Value, skip map[string]bool) {
	m, ok := v.(*bsl.Map)
	if !ok {
		return
	}
	for _, k := range m.Keys() {
		key := bsl.ToString(k)
		if skip[key] || serverFields[key] {
			continue
		}
		val, _ := m.Lookup(k)
		e.assign(target, key, val)
	}
}

func isIdent(s string) bool {
	for i, r := range s {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= 'а' && r <= 'я' || r >= 'А' && r <= 'Я' || r == 'ё' || r == 'Ё' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return s != ""
}

func (e *exporter) assign(target, key string, v bsl.Value) {
	path := target + "." + key
	if !isIdent(key) {
		switch v.(type) {
		case *bsl.Map, *bsl.Array:
			e.line("%s.Вставить(%s, %s);", target, quote(key), "Новый Соответствие")
			e.fieldsByIndex(fmt.Sprintf("%s[%s]", target, quote(key)), v)
		default:
			e.line("%s.Вставить(%s, %s);", target, quote(key), literal(v))
		}
		return
	}
	switch x := v.(type) {
	case *bsl.Map:
		e.fields(path, x, nil)
	case *bsl.Array:
		elem := elementNames[key]
		if elem == "" {
			elem = "Элемент"
		}
		for _, item := range x.Elems {
			if m, ok := item.(*bsl.Map); ok {
				e.line("%s = %s.Добавить();", elem, path)
				e.fields(elem, m, nil)
			} else {
				e.line("%s.Добавить(%s);", path, literal(item))
			}
		}
	default:
		e.line("%s = %s;", path, literal(v))
	}
}

func (e *exporter) fieldsByIndex(target string, v bsl.Value) {
	if m, ok := v.(*bsl.Map); ok {
		for _, k := range m.Keys() {
			val, _ := m.Lookup(k)
			e.line("%s[%s] = %s;", target, quote(bsl.ToString(k)), literal(val))
		}
	}
}

func quote(s string) string {
	s = strings.ReplaceAll(s, `"`, `""`)
	return `"` + strings.ReplaceAll(s, "\n", "\n|") + `"`
}

func literal(v bsl.Value) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "Истина"
		}
		return "Ложь"
	case bsl.Number:
		if x.Sign() < 0 {
			return "-" + x.Neg().Plain()
		}
		return x.Plain()
	case string:
		return quote(x)
	case time.Time:
		return "'" + x.Format("20060102150405") + "'"
	case bsl.Undefined, nil:
		return "Неопределено"
	}
	s, err := bsl.WriteJSON(v, "")
	if err != nil {
		return "Неопределено"
	}
	return quote(s)
}
