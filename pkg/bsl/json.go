package bsl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// ParseJSON reads JSON into values of the language, keeping the order of
// keys. Objects become Соответствие when toMap is set, Структура otherwise.
func ParseJSON(data []byte, toMap bool) (Value, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeJSON(dec, toMap)
	if err != nil {
		return nil, fmt.Errorf("Ошибка чтения JSON: %v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("Ошибка чтения JSON: лишние данные после значения")
	}
	return v, nil
}

func decodeJSON(dec *json.Decoder, toMap bool) (Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			var s *Structure
			var m *Map
			if toMap {
				m = NewMap()
			} else {
				s = NewStructure()
			}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k := kt.(string)
				v, err := decodeJSON(dec, toMap)
				if err != nil {
					return nil, err
				}
				if toMap {
					m.Insert(k, v)
				} else if err := s.Insert(k, v); err != nil {
					return nil, fmt.Errorf("имя свойства «%s» не годится для Структуры, читайте в Соответствие", k)
				}
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			if toMap {
				return m, nil
			}
			return s, nil
		case '[':
			a := NewArray()
			for dec.More() {
				v, err := decodeJSON(dec, toMap)
				if err != nil {
					return nil, err
				}
				a.Elems = append(a.Elems, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return a, nil
		}
	case json.Number:
		return ParseNumber(t.String())
	case string:
		return t, nil
	case bool:
		return t, nil
	case nil:
		return Undefined{}, nil
	}
	return nil, fmt.Errorf("неожиданный элемент %v", tok)
}

// WriteJSON writes a value as JSON. indent is empty for one line.
func WriteJSON(v Value, indent string) (string, error) {
	var b strings.Builder
	if err := encodeJSON(&b, v, indent, ""); err != nil {
		return "", err
	}
	return b.String(), nil
}

func encodeJSON(b *strings.Builder, v Value, indent, prefix string) error {
	nl := func(p string) {
		if indent != "" {
			b.WriteString("\n" + p)
		}
	}
	colon := ":"
	if indent != "" {
		colon = ": "
	}
	switch x := v.(type) {
	case nil, Undefined, Null:
		b.WriteString("null")
	case bool:
		fmt.Fprint(b, x)
	case Number:
		b.WriteString(x.Plain())
	case string:
		q, _ := json.Marshal(x)
		b.Write(q)
	case time.Time:
		q, _ := json.Marshal(x.Format("2006-01-02T15:04:05"))
		b.Write(q)
	case *Structure:
		b.WriteByte('{')
		for i, k := range x.order {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(prefix + indent)
			q, _ := json.Marshal(x.names[k])
			b.Write(q)
			b.WriteString(colon)
			if err := encodeJSON(b, x.vals[k], indent, prefix+indent); err != nil {
				return err
			}
		}
		if len(x.order) > 0 {
			nl(prefix)
		}
		b.WriteByte('}')
	case *Map:
		b.WriteByte('{')
		for i, k := range x.keys {
			ks, ok := k.(string)
			if !ok {
				return fmt.Errorf("Ключ соответствия для записи в JSON должен быть строкой")
			}
			if i > 0 {
				b.WriteByte(',')
			}
			nl(prefix + indent)
			q, _ := json.Marshal(ks)
			b.Write(q)
			b.WriteString(colon)
			if err := encodeJSON(b, x.vals[mapKey(k)], indent, prefix+indent); err != nil {
				return err
			}
		}
		if len(x.keys) > 0 {
			nl(prefix)
		}
		b.WriteByte('}')
	case *Array:
		b.WriteByte('[')
		for i, e := range x.Elems {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(prefix + indent)
			if err := encodeJSON(b, e, indent, prefix+indent); err != nil {
				return err
			}
		}
		if len(x.Elems) > 0 {
			nl(prefix)
		}
		b.WriteByte(']')
	case json.Marshaler:
		data, err := x.MarshalJSON()
		if err != nil {
			return err
		}
		b.Write(data)
	default:
		return fmt.Errorf("Передано значение недопустимого типа для записи JSON (%s)", TypeOf(v))
	}
	return nil
}

// ToGo converts a value to plain Go: map[string]any, []any, string,
// json.Number, bool and nil, as encoding/json would decode it.
func ToGo(v Value) (any, error) {
	s, err := WriteJSON(v, "")
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var out any
	return out, dec.Decode(&out)
}

// FromGo converts plain Go data, as encoding/json decodes it, to values;
// objects become Соответствие.
func FromGo(x any) (Value, error) {
	data, err := json.Marshal(x)
	if err != nil {
		return nil, err
	}
	return ParseJSON(data, true)
}

// jsonReader is ЧтениеJSON.
type jsonReader struct{ data []byte }

func (*jsonReader) TypeName() string { return "ЧтениеJSON" }

func (r *jsonReader) CallMethod(_ *Interp, name string, args []*Ref) (Value, error) {
	switch norm(name) {
	case "установитьстроку", "settostring":
		r.data = []byte(ToString(Arg(args, 0)))
		return nil, nil
	case "закрыть", "close":
		r.data = nil
		return nil, nil
	}
	return nil, errNoMethod(name)
}

// jsonWriter is ЗаписьJSON.
type jsonWriter struct {
	indent string
	buf    strings.Builder
	open   bool
}

func (*jsonWriter) TypeName() string { return "ЗаписьJSON" }

func (w *jsonWriter) CallMethod(_ *Interp, name string, args []*Ref) (Value, error) {
	switch norm(name) {
	case "установитьстроку", "settostring":
		w.buf.Reset()
		w.open = true
		w.indent = ""
		if p, ok := Arg(args, 0).(*jsonWriteParams); ok {
			w.indent = p.indent
		}
		return nil, nil
	case "закрыть", "close":
		s := w.buf.String()
		w.buf.Reset()
		w.open = false
		return s, nil
	}
	return nil, errNoMethod(name)
}

// jsonWriteParams is ПараметрыЗаписиJSON(ПереносСтрок, СимволОтступа).
type jsonWriteParams struct{ indent string }

func (*jsonWriteParams) TypeName() string { return "ПараметрыЗаписиJSON" }

type lineBreakEnum struct{}

func (lineBreakEnum) TypeName() string { return "ПереносСтрокJSON" }
func (lineBreakEnum) Get(name string) (Value, bool) {
	switch norm(name) {
	case "нет", "none":
		return "нет", true
	case "авто", "auto", "unix", "windows":
		return "авто", true
	}
	return nil, false
}

func registerJSON(it *Interp) {
	it.SetGlobal(lineBreakEnum{}, "ПереносСтрокJSON", "JSONLineBreak")
	it.DefineType(func(_ *Interp, _ []Value) (Value, error) { return &jsonReader{}, nil }, "ЧтениеJSON", "JSONReader")
	it.DefineType(func(_ *Interp, _ []Value) (Value, error) { return &jsonWriter{}, nil }, "ЗаписьJSON", "JSONWriter")
	it.DefineType(func(_ *Interp, args []Value) (Value, error) {
		p := &jsonWriteParams{indent: "  "}
		if len(args) > 0 && ToString(args[0]) == "нет" {
			p.indent = ""
		}
		if len(args) > 1 {
			if s, ok := args[1].(string); ok && p.indent != "" {
				p.indent = s
			}
		}
		return p, nil
	}, "ПараметрыЗаписиJSON", "JSONWriterSettings")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		r, ok := Arg(args, 0).(*jsonReader)
		if !ok {
			return nil, fmt.Errorf("Неверный тип параметра 1: ожидается ЧтениеJSON")
		}
		toMap, _ := Arg(args, 1).(bool)
		return ParseJSON(r.data, toMap)
	}, "ПрочитатьJSON", "ReadJSON")
	it.Define(func(_ *Interp, args []*Ref) (Value, error) {
		w, ok := Arg(args, 0).(*jsonWriter)
		if !ok || !w.open {
			return nil, fmt.Errorf("Запись JSON не открыта")
		}
		s, err := WriteJSON(Arg(args, 1), w.indent)
		if err != nil {
			return nil, err
		}
		w.buf.WriteString(s)
		return nil, nil
	}, "ЗаписатьJSON", "WriteJSON")
}
