package bsl

import (
	"strconv"
	"strings"
	"time"
)

// formatParams parses «ЧДЦ=2; ЧГ=0; ДФ='dd.MM.yyyy'» into folded names.
func formatParams(f string) map[string]string {
	out := map[string]string{}
	var name, val strings.Builder
	inName, quote := true, rune(0)
	flush := func() {
		n := strings.TrimSpace(name.String())
		if n != "" {
			v := strings.TrimSpace(val.String())
			if len(v) >= 2 && (v[0] == '\'' || v[0] == '"') && v[len(v)-1] == v[0] {
				v = v[1 : len(v)-1]
			}
			out[norm(n)] = v
		}
		name.Reset()
		val.Reset()
		inName = true
	}
	for _, r := range f {
		switch {
		case quote != 0:
			val.WriteRune(r)
			if r == quote {
				quote = 0
			}
		case r == ';':
			flush()
		case r == '=' && inName:
			inName = false
		case inName:
			name.WriteRune(r)
		default:
			if r == '\'' || r == '"' {
				quote = r
			}
			val.WriteRune(r)
		}
	}
	flush()
	return out
}

func param(p map[string]string, names ...string) (string, bool) {
	for _, n := range names {
		if v, ok := p[norm(n)]; ok {
			return v, true
		}
	}
	return "", false
}

// FormatNumber formats a number by a format string of the platform.
// With an empty format it is the form Строка() gives.
func FormatNumber(n Number, format string) string {
	p := formatParams(format)
	if n.Sign() == 0 {
		if z, ok := param(p, "ЧН", "NZ"); ok {
			return z
		}
		if format != "" {
			return ""
		}
	}
	decSep := ","
	if v, ok := param(p, "ЧРД", "NDS"); ok {
		decSep = v
	}
	grpSep := " "
	if v, ok := param(p, "ЧРГ", "NGS"); ok {
		grpSep = v
	}
	group := 3
	if v, ok := param(p, "ЧГ", "NG"); ok {
		if v == "0" {
			group = 0
		} else if i, err := strconv.Atoi(strings.Split(v, ",")[0]); err == nil {
			group = i
		}
	}
	var s string
	if d, ok := param(p, "ЧДЦ", "NFD"); ok {
		digits, _ := strconv.Atoi(d)
		s = n.Round(digits).rat().FloatString(digits)
	} else {
		s = n.Plain()
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, _ := strings.Cut(s, ".")
	if lz, ok := param(p, "ЧВН", "NLZ"); ok && lz != "" {
		if total, ok := param(p, "ЧЦ", "ND"); ok {
			t, _ := strconv.Atoi(total)
			fd := len(frac)
			for len(intPart) < t-fd {
				intPart = "0" + intPart
			}
		}
	}
	if group > 0 && len(intPart) > group {
		var b strings.Builder
		pre := len(intPart) % group
		if pre > 0 {
			b.WriteString(intPart[:pre])
		}
		for i := pre; i < len(intPart); i += group {
			if b.Len() > 0 {
				b.WriteString(grpSep)
			}
			b.WriteString(intPart[i : i+group])
		}
		intPart = b.String()
	}
	out := intPart
	if frac != "" {
		out += decSep + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

var monthsGen = []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
var monthsNom = []string{"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь", "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"}
var weekdays = []string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}

// FormatDate formats a date by a format string of the platform.
func FormatDate(t time.Time, format string) string {
	p := formatParams(format)
	if t.Equal(emptyDate) || t.IsZero() {
		v, _ := param(p, "ДП", "DE")
		return v
	}
	if f, ok := param(p, "ДФ", "DF"); ok {
		return datePattern(t, f)
	}
	switch v, _ := param(p, "ДЛФ", "DLF"); strings.ToUpper(v) {
	case "D", "Д":
		return t.Format("02.01.2006")
	case "DD", "ДД":
		return datePattern(t, "d MMMM yyyy 'г.'")
	case "T", "В":
		return t.Format("15:04:05")
	case "DDT", "ДДВ":
		return datePattern(t, "d MMMM yyyy 'г.' H:mm:ss")
	}
	return t.Format("02.01.2006 15:04:05")
}

func datePattern(t time.Time, f string) string {
	var b strings.Builder
	rs := []rune(f)
	for i := 0; i < len(rs); {
		r := rs[i]
		if r == '\'' {
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				j++
			}
			b.WriteString(string(rs[i+1 : min(j, len(rs))]))
			i = j + 1
			continue
		}
		n := 1
		for i+n < len(rs) && rs[i+n] == r {
			n++
		}
		switch r {
		case 'd':
			switch {
			case n == 1:
				b.WriteString(strconv.Itoa(t.Day()))
			case n == 2:
				b.WriteString(t.Format("02"))
			default:
				b.WriteString(weekdays[t.Weekday()])
			}
		case 'M':
			switch n {
			case 1:
				b.WriteString(strconv.Itoa(int(t.Month())))
			case 2:
				b.WriteString(t.Format("01"))
			case 3:
				b.WriteString(string([]rune(strings.ToLower(monthsNom[t.Month()-1]))[:3]))
			default:
				b.WriteString(monthsGen[t.Month()-1])
			}
		case 'y':
			if n <= 2 {
				b.WriteString(t.Format("06"))
			} else {
				b.WriteString(t.Format("2006"))
			}
		case 'H':
			if n == 1 {
				b.WriteString(strconv.Itoa(t.Hour()))
			} else {
				b.WriteString(t.Format("15"))
			}
		case 'h':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			if n == 1 {
				b.WriteString(strconv.Itoa(h))
			} else {
				b.WriteString(leftPad(strconv.Itoa(h), 2))
			}
		case 'm':
			if n == 1 {
				b.WriteString(strconv.Itoa(t.Minute()))
			} else {
				b.WriteString(t.Format("04"))
			}
		case 's':
			if n == 1 {
				b.WriteString(strconv.Itoa(t.Second()))
			} else {
				b.WriteString(t.Format("05"))
			}
		default:
			b.WriteString(strings.Repeat(string(r), n))
		}
		i += n
	}
	return b.String()
}

func leftPad(s string, n int) string {
	for len(s) < n {
		s = "0" + s
	}
	return s
}

// FormatBool formats a boolean by БЛ and БИ.
func FormatBool(v bool, format string) string {
	p := formatParams(format)
	if v {
		if s, ok := param(p, "БИ", "BT"); ok {
			return s
		}
		return "Да"
	}
	if s, ok := param(p, "БЛ", "BF"); ok {
		return s
	}
	return "Нет"
}
