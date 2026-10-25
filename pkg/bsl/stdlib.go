package bsl

import (
	"crypto/rand"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func str(args []*Ref, i int) string { return ToString(Arg(args, i)) }

func num(args []*Ref, i int) (Number, error) {
	switch v := Arg(args, i).(type) {
	case Number:
		return v, nil
	case Undefined:
		return NewInt(0), nil
	}
	return Number{}, fmt.Errorf("Неверный тип параметра %d: ожидается Число", i+1)
}

func intArg(args []*Ref, i int, def int) (int, error) {
	if !Has(args, i) {
		return def, nil
	}
	n, err := num(args, i)
	return int(n.Int64()), err
}

func date(args []*Ref, i int) (time.Time, error) {
	t, ok := Arg(args, i).(time.Time)
	if !ok {
		return time.Time{}, fmt.Errorf("Неверный тип параметра %d: ожидается Дата", i+1)
	}
	return t, nil
}

func runes(s string) []rune { return []rune(s) }

// Symbols is the global property Символы.
type symbols struct{}

func (symbols) TypeName() string { return "Символы" }

func (symbols) Get(name string) (Value, bool) {
	switch norm(name) {
	case "пс", "lf":
		return "\n", true
	case "вк", "cr":
		return "\r", true
	case "таб", "tab":
		return "\t", true
	case "нпп", "nbsp":
		return " ", true
	case "пф", "ff":
		return "\f", true
	case "втаб", "vtab":
		return "\v", true
	}
	return nil, false
}

func registerStdlib(it *Interp) {
	it.SetGlobal(symbols{}, "Символы", "Chars")

	it.Define(func(it *Interp, args []*Ref) (Value, error) {
		text := str(args, 0)
		if it.Message != nil {
			it.Message(text)
		} else if it.Out != nil {
			fmt.Fprintln(it.Out, text)
		}
		return nil, nil
	}, "Сообщить", "Message")

	it.Define(func(_ *Interp, _ []*Ref) (Value, error) {
		return nil, fmt.Errorf("Выполнение произвольного кода запрещено: безопасный режим")
	}, "Выполнить", "Execute", "Вычислить", "Eval")

	// Types
	it.Define(func(_ *Interp, args []*Ref) (Value, error) { return TypeValue{Name: TypeOf(Arg(args, 0))}, nil }, "ТипЗнч", "TypeOf")
	it.Define(func(it *Interp, args []*Ref) (Value, error) {
		n := strings.TrimSpace(str(args, 0))
		switch norm(n) {
		case "число", "number":
			return TypeValue{"Число"}, nil
		case "строка", "string":
			return TypeValue{"Строка"}, nil
		case "дата", "date":
			return TypeValue{"Дата"}, nil
		case "булево", "boolean":
			return TypeValue{"Булево"}, nil
		case "неопределено", "undefined":
			return TypeValue{"Неопределено"}, nil
		case "null":
			return TypeValue{"Null"}, nil
		}
		if _, ok := it.types[norm(n)]; ok {
			v, err := it.types[norm(n)](it, nil)
			if err == nil {
				return TypeValue{Name: TypeOf(v)}, nil
			}
			return TypeValue{Name: n}, nil
		}
		return nil, fmt.Errorf("Тип не определен (%s)", n)
	}, "Тип", "Type")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) { return Filled(Arg(args, 0)), nil }, "ЗначениеЗаполнено", "ValueIsFilled")

	// Conversions
	it.Define(func(_ *Interp, args []*Ref) (Value, error) { return ToString(Arg(args, 0)), nil }, "Строка", "String")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		switch v := Arg(args, 0).(type) {
		case Number:
			return v, nil
		case bool:
			if v {
				return NewInt(1), nil
			}
			return NewInt(0), nil
		case string:
			s := strings.NewReplacer(" ", "", " ", "", ",", ".").Replace(strings.TrimSpace(v))
			n, err := ParseNumber(s)
			if err != nil {
				return nil, fmt.Errorf("Преобразование значения к типу Число не может быть выполнено")
			}
			return n, nil
		}
		return nil, fmt.Errorf("Преобразование значения к типу Число не может быть выполнено")
	}, "Число", "Number")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		switch v := Arg(args, 0).(type) {
		case bool:
			return v, nil
		case Number:
			return v.Sign() != 0, nil
		case string:
			switch norm(strings.TrimSpace(v)) {
			case "да", "истина", "true", "yes", "1":
				return true, nil
			case "нет", "ложь", "false", "no", "0":
				return false, nil
			}
		}
		return nil, fmt.Errorf("Преобразование значения к типу Булево не может быть выполнено")
	}, "Булево", "Boolean")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		if s, ok := Arg(args, 0).(string); ok && len(args) == 1 {
			var d strings.Builder
			for _, r := range s {
				if unicode.IsDigit(r) {
					d.WriteRune(r)
				}
			}
			return parseDateDigits(d.String())
		}
		var p [6]int
		for i := range p {
			n, err := intArg(args, i, 0)
			if err != nil {
				return nil, err
			}
			p[i] = n
		}
		if p[1] == 0 {
			p[1] = 1
		}
		if p[2] == 0 {
			p[2] = 1
		}
		return time.Date(p[0], time.Month(p[1]), p[2], p[3], p[4], p[5], 0, time.UTC), nil
	}, "Дата", "Date")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		switch v := Arg(args, 0).(type) {
		case Number:
			return v.Plain(), nil
		case bool:
			if v {
				return "true", nil
			}
			return "false", nil
		case time.Time:
			return v.Format("2006-01-02T15:04:05"), nil
		case Undefined, nil:
			return "", nil
		}
		return ToString(Arg(args, 0)), nil
	}, "XMLСтрока", "XMLString")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		t, _ := Arg(args, 0).(TypeValue)
		s := strings.TrimSpace(str(args, 1))
		switch norm(t.Name) {
		case "дата", "date":
			for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
				if d, err := time.Parse(layout, s); err == nil {
					return time.Date(d.Year(), d.Month(), d.Day(), d.Hour(), d.Minute(), d.Second(), 0, time.UTC), nil
				}
			}
			if s == "" {
				return emptyDate, nil
			}
		case "число", "number":
			if n, err := ParseNumber(s); err == nil {
				return n, nil
			}
		case "булево", "boolean":
			switch s {
			case "true", "1":
				return true, nil
			case "false", "0":
				return false, nil
			}
		case "строка", "string":
			return s, nil
		}
		return nil, fmt.Errorf("Ошибка преобразования данных XDTO: «%s» не %s", s, t.Name)
	}, "XMLЗначение", "XMLValue")

	// Strings
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		return NewInt(int64(utf8.RuneCountInString(str(args, 0)))), nil
	}, "СтрДлина", "StrLen")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) { return strings.TrimLeft(str(args, 0), " \t\n\r "), nil }, "СокрЛ", "TrimL")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) { return strings.TrimRight(str(args, 0), " \t\n\r "), nil }, "СокрП", "TrimR")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) { return strings.Trim(str(args, 0), " \t\n\r "), nil }, "СокрЛП", "TrimAll")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		r := runes(str(args, 0))
		n, err := intArg(args, 1, 0)
		return string(r[:max(0, min(n, len(r)))]), err
	}, "Лев", "Left")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		r := runes(str(args, 0))
		n, err := intArg(args, 1, 0)
		return string(r[len(r)-max(0, min(n, len(r))):]), err
	}, "Прав", "Right")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		r := runes(str(args, 0))
		start, err := intArg(args, 1, 1)
		if err != nil {
			return nil, err
		}
		start = max(start, 1)
		if start > len(r) {
			return "", nil
		}
		n, err := intArg(args, 2, len(r)-start+1)
		end := min(start-1+max(n, 0), len(r))
		return string(r[start-1 : end]), err
	}, "Сред", "Mid")
	find := func(_ *Interp, args []*Ref) (Value, error) {
		i := strings.Index(str(args, 0), str(args, 1))
		if i < 0 {
			return NewInt(0), nil
		}
		return NewInt(int64(utf8.RuneCountInString(str(args, 0)[:i]) + 1)), nil
	}
	it.Define(find, "Найти", "Find", "СтрНайти", "StrFind")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) { return strings.ToUpper(str(args, 0)), nil }, "ВРег", "Upper")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) { return strings.ToLower(str(args, 0)), nil }, "НРег", "Lower")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		r := runes(strings.ToLower(str(args, 0)))
		up := true
		for i, c := range r {
			if up && unicode.IsLetter(c) {
				r[i] = unicode.ToUpper(c)
			}
			up = !unicode.IsLetter(c)
		}
		return string(r), nil
	}, "ТРег", "Title")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		return strings.ReplaceAll(str(args, 0), str(args, 1), str(args, 2)), nil
	}, "СтрЗаменить", "StrReplace")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		if str(args, 1) == "" {
			return NewInt(0), nil
		}
		return NewInt(int64(strings.Count(str(args, 0), str(args, 1)))), nil
	}, "СтрЧислоВхождений", "StrOccurrenceCount")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		s := str(args, 0)
		if s == "" {
			return NewInt(0), nil
		}
		return NewInt(int64(strings.Count(s, "\n") + 1)), nil
	}, "СтрЧислоСтрок", "StrLineCount")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		lines := strings.Split(str(args, 0), "\n")
		n, err := intArg(args, 1, 1)
		if err != nil || n < 1 || n > len(lines) {
			return "", err
		}
		return strings.TrimSuffix(lines[n-1], "\r"), nil
	}, "СтрПолучитьСтроку", "StrGetLine")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		n, err := num(args, 0)
		return string(rune(n.Int64())), err
	}, "Символ", "Char")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		r := runes(str(args, 0))
		i, err := intArg(args, 1, 1)
		if err != nil || i < 1 || i > len(r) {
			return NewInt(-1), err
		}
		return NewInt(int64(r[i-1])), nil
	}, "КодСимвола", "CharCode")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		return strings.TrimSpace(str(args, 0)) == "", nil
	}, "ПустаяСтрока", "IsBlankString")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		tmpl := runes(str(args, 0))
		var b strings.Builder
		for i := 0; i < len(tmpl); i++ {
			if tmpl[i] != '%' || i+1 >= len(tmpl) {
				b.WriteRune(tmpl[i])
				continue
			}
			if tmpl[i+1] == '%' {
				b.WriteRune('%')
				i++
				continue
			}
			j := i + 1
			paren := tmpl[j] == '('
			if paren {
				j++
			}
			k := j
			for k < len(tmpl) && unicode.IsDigit(tmpl[k]) && (paren || k == j) {
				k++
			}
			if k == j {
				b.WriteRune('%')
				continue
			}
			var idx int
			fmt.Sscan(string(tmpl[j:k]), &idx)
			if paren {
				if k >= len(tmpl) || tmpl[k] != ')' {
					return nil, fmt.Errorf("Неверный шаблон строки")
				}
				k++
			}
			if idx < 1 || idx > 10 {
				return nil, fmt.Errorf("Неверный номер параметра шаблона: %d", idx)
			}
			b.WriteString(str(args, idx))
			i = k - 1
		}
		return b.String(), nil
	}, "СтрШаблон", "StrTemplate")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		s, seps := str(args, 0), str(args, 1)
		includeEmpty := true
		if b, ok := Arg(args, 2).(bool); ok {
			includeEmpty = b
		}
		if !Has(args, 1) {
			seps = ","
		}
		parts := strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(seps, r) })
		if includeEmpty {
			parts = splitAny(s, seps)
		}
		out := NewArray()
		for _, p := range parts {
			out.Elems = append(out.Elems, p)
		}
		return out, nil
	}, "СтрРазделить", "StrSplit")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		var parts []string
		switch a := Arg(args, 0).(type) {
		case *Array:
			for _, e := range a.Elems {
				parts = append(parts, ToString(e))
			}
		default:
			return nil, fmt.Errorf("Неверный тип параметра 1: ожидается Массив")
		}
		return strings.Join(parts, str(args, 1)), nil
	}, "СтрСоединить", "StrConcat")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		return strings.HasPrefix(str(args, 0), str(args, 1)), nil
	}, "СтрНачинаетсяС", "StrStartsWith")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		return strings.HasSuffix(str(args, 0), str(args, 1)), nil
	}, "СтрЗаканчиваетсяНа", "StrEndsWith")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		return NewInt(int64(strings.Compare(strings.ToLower(str(args, 0)), strings.ToLower(str(args, 1))))), nil
	}, "СтрСравнить", "StrCompare")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		r := runes(str(args, 1))
		n, err := intArg(args, 0, 0)
		if err != nil || len(r) == 0 {
			return "", err
		}
		return strings.Repeat(string(r), max(n, 0)), nil
	}, "СтрПовторить")

	// Numbers
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		n, err := num(args, 0)
		return n.Trunc(), err
	}, "Цел", "Int")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		n, err := num(args, 0)
		if err != nil {
			return nil, err
		}
		d, err := intArg(args, 1, 0)
		return n.Round(d), err
	}, "Окр", "Round")
	minmax := func(less bool) Builtin {
		return func(_ *Interp, args []*Ref) (Value, error) {
			if len(args) == 0 {
				return nil, fmt.Errorf("Недостаточно фактических параметров")
			}
			best := Arg(args, 0)
			for i := 1; i < len(args); i++ {
				c, err := Compare(Arg(args, i), best)
				if err != nil {
					return nil, err
				}
				if less && c < 0 || !less && c > 0 {
					best = Arg(args, i)
				}
			}
			return best, nil
		}
	}
	it.Define(minmax(true), "Мин", "Min")
	it.Define(minmax(false), "Макс", "Max")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		n, err := num(args, 0)
		if n.Sign() < 0 {
			return n.Neg(), err
		}
		return n, err
	}, "Abs")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		b, err := num(args, 0)
		if err != nil {
			return nil, err
		}
		e, err := num(args, 1)
		if err != nil {
			return nil, err
		}
		if e.IsInt() && e.Sign() >= 0 {
			r := NewInt(1)
			for i := int64(0); i < e.Int64(); i++ {
				r = r.Mul(b)
			}
			return r, nil
		}
		return NewFloat(math.Pow(b.Float64(), e.Float64())), nil
	}, "Pow")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		n, err := num(args, 0)
		return NewFloat(math.Sqrt(n.Float64())), err
	}, "Sqrt")

	// Formatting
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		f := str(args, 1)
		switch v := Arg(args, 0).(type) {
		case Number:
			return FormatNumber(v, f), nil
		case time.Time:
			return FormatDate(v, f), nil
		case bool:
			return FormatBool(v, f), nil
		}
		return ToString(Arg(args, 0)), nil
	}, "Формат", "Format")

	registerDates(it)
	registerErrors(it)
	registerCollections(it)
	registerJSON(it)
}

func splitAny(s, seps string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if strings.ContainsRune(seps, r) {
			out = append(out, s[start:i])
			start = i + utf8.RuneLen(r)
		}
	}
	return append(out, s[start:])
}

func registerDates(it *Interp) {
	it.Define(func(it *Interp, _ []*Ref) (Value, error) { return localNow(it), nil }, "ТекущаяДата", "CurrentDate")
	it.Define(func(it *Interp, _ []*Ref) (Value, error) { return localNow(it), nil }, "ТекущаяДатаСеанса", "CurrentSessionDate")
	it.Define(func(it *Interp, args []*Ref) (Value, error) {
		t, err := date(args, 0)
		if err != nil {
			return nil, err
		}
		zone := str(args, 1)
		if zone == "" {
			zone = "Europe/Moscow"
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			return nil, fmt.Errorf("Неизвестный часовой пояс (%s)", zone)
		}
		l := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC).In(loc)
		return time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute(), l.Second(), 0, time.UTC), nil
	}, "МестноеВремя", "ToLocalTime")
	it.Define(func(it *Interp, _ []*Ref) (Value, error) {
		n := it.Now().UTC()
		return time.Date(n.Year(), n.Month(), n.Day(), n.Hour(), n.Minute(), n.Second(), 0, time.UTC), nil
	}, "ТекущаяУниверсальнаяДата", "CurrentUniversalDate")
	part := func(get func(time.Time) int) Builtin {
		return func(_ *Interp, args []*Ref) (Value, error) {
			t, err := date(args, 0)
			return NewInt(int64(get(t))), err
		}
	}
	it.Define(part(func(t time.Time) int { return t.Year() }), "Год", "Year")
	it.Define(part(func(t time.Time) int { return int(t.Month()) }), "Месяц", "Month")
	it.Define(part(func(t time.Time) int { return t.Day() }), "День", "Day")
	it.Define(part(func(t time.Time) int { return t.Hour() }), "Час", "Hour")
	it.Define(part(func(t time.Time) int { return t.Minute() }), "Минута", "Minute")
	it.Define(part(func(t time.Time) int { return t.Second() }), "Секунда", "Second")
	it.Define(part(func(t time.Time) int { return t.YearDay() }), "ДеньГода", "DayOfYear")
	it.Define(part(func(t time.Time) int { return (int(t.Weekday())+6)%7 + 1 }), "ДеньНедели", "WeekDay")
	conv := func(f func(time.Time) time.Time) Builtin {
		return func(_ *Interp, args []*Ref) (Value, error) {
			t, err := date(args, 0)
			return f(t), err
		}
	}
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	month := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC) }
	quarter := func(t time.Time) time.Time {
		return time.Date(t.Year(), time.Month((int(t.Month())-1)/3*3+1), 1, 0, 0, 0, 0, time.UTC)
	}
	year := func(t time.Time) time.Time { return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC) }
	end := func(start func(time.Time) time.Time, next func(time.Time) time.Time) func(time.Time) time.Time {
		return func(t time.Time) time.Time { return next(start(t)).Add(-time.Second) }
	}
	it.Define(conv(day), "НачалоДня", "BegOfDay")
	it.Define(conv(end(day, func(t time.Time) time.Time { return t.AddDate(0, 0, 1) })), "КонецДня", "EndOfDay")
	it.Define(conv(month), "НачалоМесяца", "BegOfMonth")
	it.Define(conv(end(month, func(t time.Time) time.Time { return t.AddDate(0, 1, 0) })), "КонецМесяца", "EndOfMonth")
	it.Define(conv(quarter), "НачалоКвартала", "BegOfQuarter")
	it.Define(conv(end(quarter, func(t time.Time) time.Time { return t.AddDate(0, 3, 0) })), "КонецКвартала", "EndOfQuarter")
	it.Define(conv(year), "НачалоГода", "BegOfYear")
	it.Define(conv(end(year, func(t time.Time) time.Time { return t.AddDate(1, 0, 0) })), "КонецГода", "EndOfYear")
	it.Define(conv(func(t time.Time) time.Time {
		d := day(t)
		return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
	}), "НачалоНедели", "BegOfWeek")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		t, err := date(args, 0)
		if err != nil {
			return nil, err
		}
		n, err := intArg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		// 31 января + 1 месяц = 28 (29) февраля, как в платформе
		y, m := t.Year(), int(t.Month())-1+n
		y += m / 12
		m %= 12
		if m < 0 {
			m += 12
			y--
		}
		last := time.Date(y, time.Month(m+2), 0, 0, 0, 0, 0, time.UTC).Day()
		return time.Date(y, time.Month(m+1), min(t.Day(), last), t.Hour(), t.Minute(), t.Second(), 0, time.UTC), nil
	}, "ДобавитьМесяц", "AddMonth")
	it.Define(func(it *Interp, args []*Ref) (Value, error) {
		ms, err := intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		select {
		case <-time.After(time.Duration(ms) * time.Millisecond):
		case <-it.ctx.Done():
			return nil, fmt.Errorf("Выполнение прервано: %v", it.ctx.Err())
		}
		return nil, nil
	}, "Приостановить", "Sleep")
	it.Define(func(it *Interp, args []*Ref) (Value, error) {
		if it.Env == nil {
			return Undefined{}, nil
		}
		if v, ok := it.Env(str(args, 0)); ok {
			return v, nil
		}
		return Undefined{}, nil
	}, "ПолучитьПеременнуюСреды", "GetEnvironmentVariable")
}

// localNow is the clock as a Дата: wall time without a zone.
func localNow(it *Interp) time.Time {
	n := it.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), n.Hour(), n.Minute(), n.Second(), 0, time.UTC)
}

// ErrorInfo is ИнформацияОбОшибке.
type ErrorInfo struct{ ex *Exception }

func (*ErrorInfo) TypeName() string { return "ИнформацияОбОшибке" }

func (e *ErrorInfo) Get(name string) (Value, bool) {
	if e.ex == nil {
		return Undefined{}, true
	}
	switch norm(name) {
	case "описание", "description":
		return e.ex.Description, true
	case "имямодуля", "modulename":
		return e.ex.Module, true
	case "номерстроки", "linenumber":
		return NewInt(int64(e.ex.Line)), true
	case "причина", "cause":
		if e.ex.Cause == nil {
			return Undefined{}, true
		}
		return &ErrorInfo{ex: e.ex.Cause}, true
	}
	return nil, false
}

func registerErrors(it *Interp) {
	it.Define(func(it *Interp, _ []*Ref) (Value, error) {
		if it.cur == nil || it.cur.exc == nil {
			return &ErrorInfo{}, nil
		}
		return &ErrorInfo{ex: it.cur.exc}, nil
	}, "ИнформацияОбОшибке", "ErrorInfo")
	it.Define(func(it *Interp, _ []*Ref) (Value, error) {
		if it.cur == nil || it.cur.exc == nil {
			return "", nil
		}
		return it.cur.exc.Error(), nil
	}, "ОписаниеОшибки", "ErrorDescription")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		if e, ok := Arg(args, 0).(*ErrorInfo); ok && e.ex != nil {
			return e.ex.Description, nil
		}
		return "", nil
	}, "КраткоеПредставлениеОшибки", "BriefErrorDescription")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		if e, ok := Arg(args, 0).(*ErrorInfo); ok && e.ex != nil {
			return e.ex.Error(), nil
		}
		return "", nil
	}, "ПодробноеПредставлениеОшибки", "DetailErrorDescription")
}

func registerCollections(it *Interp) {
	it.DefineType(func(_ *Interp, args []Value) (Value, error) {
		if len(args) == 1 {
			if s, ok := args[0].(*Structure); ok {
				return s.copy(), nil
			}
		}
		s := NewStructure()
		if len(args) == 0 {
			return s, nil
		}
		keys, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("Неверный тип параметра 1: ожидается строка ключей")
		}
		for i, k := range splitCols(keys) {
			var v Value = Undefined{}
			if i+1 < len(args) {
				v = args[i+1]
			}
			if err := s.Insert(k, v); err != nil {
				return nil, err
			}
		}
		return s, nil
	}, "Структура", "Structure")
	it.DefineType(func(it *Interp, args []Value) (Value, error) {
		v, err := it.types["структура"](it, args)
		if err != nil {
			return nil, err
		}
		return v.(*Structure).Fixed(), nil
	}, "ФиксированнаяСтруктура", "FixedStructure")
	it.DefineType(func(_ *Interp, args []Value) (Value, error) {
		m := NewMap()
		if len(args) == 1 {
			switch src := args[0].(type) {
			case *Map:
				for _, k := range src.keys {
					m.Insert(k, src.vals[mapKey(k)])
				}
			case *Structure:
				for _, k := range src.order {
					m.Insert(src.names[k], src.vals[k])
				}
			}
		}
		return m, nil
	}, "Соответствие", "Map")
	it.DefineType(func(it *Interp, args []Value) (Value, error) {
		v, _ := it.types["соответствие"](it, args)
		return v.(*Map).Fixed(), nil
	}, "ФиксированноеСоответствие", "FixedMap")
	it.DefineType(func(_ *Interp, args []Value) (Value, error) {
		a := NewArray()
		if len(args) == 1 {
			switch x := args[0].(type) {
			case Number:
				n := int(x.Int64())
				if n < 0 {
					return nil, errIndex()
				}
				a.Elems = make([]Value, n)
				for i := range a.Elems {
					a.Elems[i] = Undefined{}
				}
			case *Array:
				a.Elems = append(a.Elems, x.Elems...)
			}
		}
		return a, nil
	}, "Массив", "Array")
	it.DefineType(func(_ *Interp, args []Value) (Value, error) {
		a := NewArray()
		if len(args) == 1 {
			if src, ok := args[0].(*Array); ok {
				a.Elems = append(a.Elems, src.Elems...)
			}
		}
		return a.Fixed(), nil
	}, "ФиксированныйМассив", "FixedArray")
	it.DefineType(func(_ *Interp, _ []Value) (Value, error) { return NewValueTable(), nil }, "ТаблицаЗначений", "ValueTable")
	it.DefineType(func(_ *Interp, args []Value) (Value, error) {
		if len(args) > 0 {
			return &UUID{s: ToString(args[0])}, nil
		}
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}
		b[6] = b[6]&0x0f | 0x40
		b[8] = b[8]&0x3f | 0x80
		return &UUID{s: fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])}, nil
	}, "УникальныйИдентификатор", "UUID")
}

// UUID is УникальныйИдентификатор; Строка() gives its text.
type UUID struct{ s string }

func (*UUID) TypeName() string { return "УникальныйИдентификатор" }
func (u *UUID) String() string { return u.s }
func (u *UUID) EqualTo(v Value) bool {
	o, ok := v.(*UUID)
	return ok && o.s == u.s
}
