package zapros

import (
	"fmt"
	"strings"

	"github.com/tym83/assnetes/pkg/bsl"
)

// Register defines Запрос in a context of the language, over data.
func Register(it *bsl.Interp, data Data) {
	it.DefineType(func(_ *bsl.Interp, args []bsl.Value) (bsl.Value, error) {
		q := &query{data: data, params: map[string]bsl.Value{}}
		if len(args) > 0 {
			q.text = bsl.ToString(args[0])
		}
		return q, nil
	}, "Запрос", "Query")
}

// query is Запрос.
type query struct {
	data   Data
	text   string
	params map[string]bsl.Value
}

func (*query) TypeName() string { return "Запрос" }

func (q *query) Get(name string) (bsl.Value, bool) {
	if strings.EqualFold(name, "Текст") || strings.EqualFold(name, "Text") {
		return q.text, true
	}
	return nil, false
}

func (q *query) Set(name string, v bsl.Value) error {
	if strings.EqualFold(name, "Текст") || strings.EqualFold(name, "Text") {
		q.text = bsl.ToString(v)
		return nil
	}
	return fmt.Errorf("Поле объекта не обнаружено (%s)", name)
}

func (q *query) CallMethod(_ *bsl.Interp, name string, args []*bsl.Ref) (bsl.Value, error) {
	switch strings.ToLower(name) {
	case "установитьпараметр", "setparameter":
		q.params[bsl.ToString(bsl.Arg(args, 0))] = bsl.Arg(args, 1)
		return nil, nil
	case "выполнить", "execute":
		parsed, err := Parse(q.text)
		if err != nil {
			return nil, err
		}
		t, err := Execute(parsed, q.data, q.params)
		if err != nil {
			return nil, err
		}
		return &result{t: t}, nil
	}
	return nil, fmt.Errorf("Метод объекта не обнаружен (%s)", name)
}

// result is РезультатЗапроса.
type result struct{ t *bsl.ValueTable }

func (*result) TypeName() string { return "РезультатЗапроса" }

func (r *result) CallMethod(_ *bsl.Interp, name string, _ []*bsl.Ref) (bsl.Value, error) {
	switch strings.ToLower(name) {
	case "выгрузить", "unload":
		return r.t, nil
	case "пустой", "isempty":
		return r.t.Count() == 0, nil
	case "выбрать", "select":
		return &selection{t: r.t, pos: -1}, nil
	}
	return nil, fmt.Errorf("Метод объекта не обнаружен (%s)", name)
}

// selection is ВыборкаИзРезультатаЗапроса.
type selection struct {
	t   *bsl.ValueTable
	pos int
}

func (*selection) TypeName() string { return "ВыборкаИзРезультатаЗапроса" }

func (s *selection) CallMethod(_ *bsl.Interp, name string, _ []*bsl.Ref) (bsl.Value, error) {
	switch strings.ToLower(name) {
	case "следующий", "next":
		if s.pos+1 >= s.t.Count() {
			return false, nil
		}
		s.pos++
		return true, nil
	case "количество", "count":
		return bsl.NewInt(int64(s.t.Count())), nil
	}
	return nil, fmt.Errorf("Метод объекта не обнаружен (%s)", name)
}

func (s *selection) Get(name string) (bsl.Value, bool) {
	if s.pos < 0 || s.pos >= s.t.Count() {
		return nil, false
	}
	return s.t.Rows()[s.pos].Get(name)
}
