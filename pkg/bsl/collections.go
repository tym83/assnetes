package bsl

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// methodTable maps every spelling of a method, Russian and English, folded,
// to one id.
type methodTable map[string]string

func methods(pairs ...string) methodTable {
	t := methodTable{}
	for i := 0; i < len(pairs); i += 2 {
		t[norm(pairs[i])] = norm(pairs[i])
		t[norm(pairs[i+1])] = norm(pairs[i])
	}
	return t
}

func (t methodTable) id(name string) string { return t[norm(name)] }

func errNoMethod(name string) error {
	return fmt.Errorf("Метод объекта не обнаружен (%s)", name)
}
func errNoField(name string) error {
	return fmt.Errorf("Поле объекта не обнаружено (%s)", name)
}
func errReadOnly() error {
	return fmt.Errorf("Объект недоступен для изменения")
}
func errIndex() error {
	return fmt.Errorf("Индекс находится за границами диапазона")
}

func argCount(args []*Ref, min, max int) error {
	if len(args) < min {
		return fmt.Errorf("Недостаточно фактических параметров")
	}
	if max >= 0 && len(args) > max {
		return fmt.Errorf("Слишком много фактических параметров")
	}
	return nil
}

// isIdentifier reports whether s can be a key of a Структура.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && !isIdentStart(r) || !isIdentPart(r) {
			return false
		}
	}
	return true
}

// Structure is Структура (and ФиксированнаяСтруктура when fixed): keys are
// identifiers, compared in any case, kept in the order they were added.
type Structure struct {
	order []string // folded keys
	names map[string]string
	vals  map[string]Value
	fixed bool
}

func NewStructure() *Structure {
	return &Structure{names: map[string]string{}, vals: map[string]Value{}}
}

func (s *Structure) TypeName() string {
	if s.fixed {
		return "ФиксированнаяСтруктура"
	}
	return "Структура"
}

func (s *Structure) Count() int { return len(s.order) }

// Keys returns the keys as they were written.
func (s *Structure) Keys() []string {
	out := make([]string, len(s.order))
	for i, k := range s.order {
		out[i] = s.names[k]
	}
	return out
}

// Insert adds or replaces a key; it is Вставить().
func (s *Structure) Insert(key string, v Value) error {
	key = strings.TrimSpace(key)
	if !isIdentifier(key) {
		return fmt.Errorf("Задано неправильное имя атрибута структуры (%s)", key)
	}
	k := norm(key)
	if _, ok := s.vals[k]; !ok {
		s.order = append(s.order, k)
		s.names[k] = key
	}
	if v == nil {
		v = Undefined{}
	}
	s.vals[k] = v
	return nil
}

func (s *Structure) Get(name string) (Value, bool) {
	v, ok := s.vals[norm(name)]
	return v, ok
}

func (s *Structure) Set(name string, v Value) error {
	if s.fixed {
		return errReadOnly()
	}
	if _, ok := s.vals[norm(name)]; !ok {
		return errNoField(name)
	}
	s.vals[norm(name)] = v
	return nil
}

func (s *Structure) GetIndex(key Value) (Value, error) {
	k, ok := key.(string)
	if !ok {
		return nil, fmt.Errorf("Неверный тип ключа: ожидается строка")
	}
	if v, ok := s.Get(k); ok {
		return v, nil
	}
	return nil, errNoField(k)
}

func (s *Structure) SetIndex(key, v Value) error {
	k, ok := key.(string)
	if !ok {
		return fmt.Errorf("Неверный тип ключа: ожидается строка")
	}
	return s.Set(k, v)
}

func (s *Structure) Items() []Value {
	out := make([]Value, len(s.order))
	for i, k := range s.order {
		out[i] = &KeyValue{Key: s.names[k], Value: s.vals[k]}
	}
	return out
}

var structureMethods = methods("Вставить", "Insert", "Свойство", "Property", "Удалить", "Delete",
	"Очистить", "Clear", "Количество", "Count")

func (s *Structure) CallMethod(_ *Interp, name string, args []*Ref) (Value, error) {
	id := structureMethods.id(name)
	if s.fixed && id != "свойство" && id != "количество" && id != "" {
		return nil, errReadOnly()
	}
	switch id {
	case "вставить":
		if err := argCount(args, 1, 2); err != nil {
			return nil, err
		}
		k, ok := Arg(args, 0).(string)
		if !ok {
			return nil, fmt.Errorf("Неверный тип ключа: ожидается строка")
		}
		return nil, s.Insert(k, Arg(args, 1))
	case "свойство":
		if err := argCount(args, 1, 2); err != nil {
			return nil, err
		}
		k, _ := Arg(args, 0).(string)
		v, ok := s.Get(k)
		if len(args) > 1 && args[1] != nil {
			if !ok {
				v = Undefined{}
			}
			args[1].V = v
		}
		return ok, nil
	case "удалить":
		k, _ := Arg(args, 0).(string)
		nk := norm(k)
		if _, ok := s.vals[nk]; ok {
			delete(s.vals, nk)
			delete(s.names, nk)
			for i, o := range s.order {
				if o == nk {
					s.order = append(s.order[:i], s.order[i+1:]...)
					break
				}
			}
		}
		return nil, nil
	case "очистить":
		s.order, s.names, s.vals = nil, map[string]string{}, map[string]Value{}
		return nil, nil
	case "количество":
		return NewInt(int64(len(s.order))), nil
	}
	return nil, errNoMethod(name)
}

// Fixed returns a fixed copy.
func (s *Structure) Fixed() *Structure {
	c := s.copy()
	c.fixed = true
	return c
}

func (s *Structure) copy() *Structure {
	c := NewStructure()
	for _, k := range s.order {
		c.order = append(c.order, k)
		c.names[k] = s.names[k]
		c.vals[k] = s.vals[k]
	}
	return c
}

// KeyValue is КлючИЗначение, what Для Каждого gives over Структура and
// Соответствие.
type KeyValue struct {
	Key   Value
	Value Value
}

func (*KeyValue) TypeName() string { return "КлючИЗначение" }

func (kv *KeyValue) Get(name string) (Value, bool) {
	switch norm(name) {
	case "ключ", "key":
		return kv.Key, true
	case "значение", "value":
		return kv.Value, true
	}
	return nil, false
}

// Map is Соответствие (ФиксированноеСоответствие when fixed): any value can
// be a key; the order of insertion is kept.
type Map struct {
	keys  []Value
	vals  map[string]Value
	index map[string]int
	fixed bool
}

func NewMap() *Map { return &Map{vals: map[string]Value{}, index: map[string]int{}} }

func (m *Map) TypeName() string {
	if m.fixed {
		return "ФиксированноеСоответствие"
	}
	return "Соответствие"
}

func mapKey(v Value) string {
	switch x := v.(type) {
	case nil, Undefined:
		return "u:"
	case Null:
		return "n:"
	case bool:
		return fmt.Sprintf("b:%v", x)
	case Number:
		return "d:" + x.Plain()
	case string:
		return "s:" + x
	case time.Time:
		return "t:" + x.Format(time.RFC3339Nano)
	case TypeValue:
		return "y:" + norm(x.Name)
	}
	return fmt.Sprintf("o:%p", v)
}

func (m *Map) Count() int { return len(m.keys) }

// Keys returns the keys in the order they were inserted.
func (m *Map) Keys() []Value { return append([]Value(nil), m.keys...) }

func (m *Map) Insert(k, v Value) {
	if v == nil {
		v = Undefined{}
	}
	mk := mapKey(k)
	if _, ok := m.index[mk]; !ok {
		m.index[mk] = len(m.keys)
		m.keys = append(m.keys, k)
	}
	m.vals[mk] = v
}

func (m *Map) Lookup(k Value) (Value, bool) {
	v, ok := m.vals[mapKey(k)]
	return v, ok
}

func (m *Map) remove(k Value) {
	mk := mapKey(k)
	i, ok := m.index[mk]
	if !ok {
		return
	}
	m.keys = append(m.keys[:i], m.keys[i+1:]...)
	delete(m.vals, mk)
	delete(m.index, mk)
	for j := i; j < len(m.keys); j++ {
		m.index[mapKey(m.keys[j])] = j
	}
}

func (m *Map) GetIndex(k Value) (Value, error) {
	if v, ok := m.Lookup(k); ok {
		return v, nil
	}
	return Undefined{}, nil
}

func (m *Map) SetIndex(k, v Value) error {
	if m.fixed {
		return errReadOnly()
	}
	m.Insert(k, v)
	return nil
}

func (m *Map) Items() []Value {
	out := make([]Value, len(m.keys))
	for i, k := range m.keys {
		out[i] = &KeyValue{Key: k, Value: m.vals[mapKey(k)]}
	}
	return out
}

var mapMethods = methods("Вставить", "Insert", "Получить", "Get", "Удалить", "Delete",
	"Очистить", "Clear", "Количество", "Count")

func (m *Map) CallMethod(_ *Interp, name string, args []*Ref) (Value, error) {
	id := mapMethods.id(name)
	if m.fixed && id != "получить" && id != "количество" && id != "" {
		return nil, errReadOnly()
	}
	switch id {
	case "вставить":
		if err := argCount(args, 1, 2); err != nil {
			return nil, err
		}
		m.Insert(Arg(args, 0), Arg(args, 1))
		return nil, nil
	case "получить":
		return m.GetIndex(Arg(args, 0))
	case "удалить":
		m.remove(Arg(args, 0))
		return nil, nil
	case "очистить":
		m.keys, m.vals, m.index = nil, map[string]Value{}, map[string]int{}
		return nil, nil
	case "количество":
		return NewInt(int64(len(m.keys))), nil
	}
	return nil, errNoMethod(name)
}

func (m *Map) Fixed() *Map {
	c := NewMap()
	for _, k := range m.keys {
		c.Insert(k, m.vals[mapKey(k)])
	}
	c.fixed = true
	return c
}

// Array is Массив (ФиксированныйМассив when fixed), indexed from zero.
type Array struct {
	Elems []Value
	fixed bool
}

func NewArray(vals ...Value) *Array { return &Array{Elems: vals} }

func (a *Array) TypeName() string {
	if a.fixed {
		return "ФиксированныйМассив"
	}
	return "Массив"
}

func (a *Array) Count() int     { return len(a.Elems) }
func (a *Array) Items() []Value { return append([]Value(nil), a.Elems...) }

func (a *Array) index(k Value) (int, error) {
	n, ok := k.(Number)
	if !ok {
		return 0, fmt.Errorf("Неверный тип индекса: ожидается число")
	}
	i := int(n.Int64())
	if i < 0 || i >= len(a.Elems) {
		return 0, errIndex()
	}
	return i, nil
}

func (a *Array) GetIndex(k Value) (Value, error) {
	i, err := a.index(k)
	if err != nil {
		return nil, err
	}
	return a.Elems[i], nil
}

func (a *Array) SetIndex(k, v Value) error {
	if a.fixed {
		return errReadOnly()
	}
	i, err := a.index(k)
	if err != nil {
		return err
	}
	a.Elems[i] = v
	return nil
}

var arrayMethods = methods("Добавить", "Add", "Вставить", "Insert", "Количество", "Count",
	"ВГраница", "UBound", "Найти", "Find", "Удалить", "Delete", "Очистить", "Clear",
	"Получить", "Get", "Установить", "Set")

func (a *Array) CallMethod(_ *Interp, name string, args []*Ref) (Value, error) {
	id := arrayMethods.id(name)
	switch id {
	case "количество":
		return NewInt(int64(len(a.Elems))), nil
	case "вграница":
		return NewInt(int64(len(a.Elems) - 1)), nil
	case "найти":
		for i, e := range a.Elems {
			if Equal(e, Arg(args, 0)) {
				return NewInt(int64(i)), nil
			}
		}
		return Undefined{}, nil
	case "получить":
		return a.GetIndex(Arg(args, 0))
	}
	if a.fixed && id != "" {
		return nil, errReadOnly()
	}
	switch id {
	case "добавить":
		a.Elems = append(a.Elems, Arg(args, 0))
		return nil, nil
	case "вставить":
		n, ok := Arg(args, 0).(Number)
		i := int(n.Int64())
		if !ok || i < 0 || i > len(a.Elems) {
			return nil, errIndex()
		}
		a.Elems = append(a.Elems[:i], append([]Value{Arg(args, 1)}, a.Elems[i:]...)...)
		return nil, nil
	case "удалить":
		i, err := a.index(Arg(args, 0))
		if err != nil {
			return nil, err
		}
		a.Elems = append(a.Elems[:i], a.Elems[i+1:]...)
		return nil, nil
	case "очистить":
		a.Elems = nil
		return nil, nil
	case "установить":
		return nil, a.SetIndex(Arg(args, 0), Arg(args, 1))
	}
	return nil, errNoMethod(name)
}

func (a *Array) Fixed() *Array {
	return &Array{Elems: append([]Value(nil), a.Elems...), fixed: true}
}

// ValueTable is ТаблицаЗначений.
type ValueTable struct {
	cols []string // as written
	rows []*ValueTableRow
}

func NewValueTable() *ValueTable { return &ValueTable{} }

func (*ValueTable) TypeName() string { return "ТаблицаЗначений" }

func (t *ValueTable) Count() int { return len(t.rows) }

func (t *ValueTable) Items() []Value {
	out := make([]Value, len(t.rows))
	for i, r := range t.rows {
		out[i] = r
	}
	return out
}

// Columns returns the column names as written.
func (t *ValueTable) Columns() []string { return append([]string(nil), t.cols...) }

// Rows returns the rows.
func (t *ValueTable) Rows() []*ValueTableRow { return t.rows }

func (t *ValueTable) hasCol(name string) bool {
	for _, c := range t.cols {
		if norm(c) == norm(name) {
			return true
		}
	}
	return false
}

// AddColumn adds a column; it is Колонки.Добавить().
func (t *ValueTable) AddColumn(name string) error {
	if !isIdentifier(name) {
		return fmt.Errorf("Неверное имя колонки (%s)", name)
	}
	if t.hasCol(name) {
		return fmt.Errorf("Колонка с таким именем уже существует (%s)", name)
	}
	t.cols = append(t.cols, name)
	return nil
}

// AddRow adds an empty row; it is Добавить().
func (t *ValueTable) AddRow() *ValueTableRow {
	r := &ValueTableRow{table: t, vals: map[string]Value{}}
	t.rows = append(t.rows, r)
	return r
}

func (t *ValueTable) GetIndex(k Value) (Value, error) {
	n, ok := k.(Number)
	if !ok {
		return nil, fmt.Errorf("Неверный тип индекса: ожидается число")
	}
	i := int(n.Int64())
	if i < 0 || i >= len(t.rows) {
		return nil, errIndex()
	}
	return t.rows[i], nil
}

func (t *ValueTable) SetIndex(Value, Value) error { return errReadOnly() }

func (t *ValueTable) Get(name string) (Value, bool) {
	if norm(name) == "колонки" || norm(name) == "columns" {
		return &tableColumns{t}, true
	}
	return nil, false
}

func (t *ValueTable) rowIndex(v Value) (int, error) {
	switch x := v.(type) {
	case Number:
		i := int(x.Int64())
		if i < 0 || i >= len(t.rows) {
			return 0, errIndex()
		}
		return i, nil
	case *ValueTableRow:
		for i, r := range t.rows {
			if r == x {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("Строка не принадлежит таблице значений")
}

func splitCols(s string) []string {
	var out []string
	for _, c := range strings.Split(s, ",") {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}

var tableMethods = methods("Добавить", "Add", "Вставить", "Insert", "Количество", "Count",
	"Получить", "Get", "Индекс", "IndexOf", "Удалить", "Delete", "Очистить", "Clear",
	"Найти", "Find", "НайтиСтроки", "FindRows", "Сортировать", "Sort", "Свернуть", "GroupBy",
	"ВыгрузитьКолонку", "UnloadColumn", "ЗагрузитьКолонку", "LoadColumn", "Итог", "Total",
	"Скопировать", "Copy")

func (t *ValueTable) CallMethod(it *Interp, name string, args []*Ref) (Value, error) {
	switch tableMethods.id(name) {
	case "добавить":
		return t.AddRow(), nil
	case "вставить":
		n, _ := Arg(args, 0).(Number)
		i := int(n.Int64())
		if i < 0 || i > len(t.rows) {
			return nil, errIndex()
		}
		r := &ValueTableRow{table: t, vals: map[string]Value{}}
		t.rows = append(t.rows[:i], append([]*ValueTableRow{r}, t.rows[i:]...)...)
		return r, nil
	case "количество":
		return NewInt(int64(len(t.rows))), nil
	case "получить":
		return t.GetIndex(Arg(args, 0))
	case "индекс":
		i, err := t.rowIndex(Arg(args, 0))
		if err != nil {
			return NewInt(-1), nil
		}
		return NewInt(int64(i)), nil
	case "удалить":
		i, err := t.rowIndex(Arg(args, 0))
		if err != nil {
			return nil, err
		}
		t.rows = append(t.rows[:i], t.rows[i+1:]...)
		return nil, nil
	case "очистить":
		t.rows = nil
		return nil, nil
	case "найти":
		cols := t.cols
		if s, ok := Arg(args, 1).(string); ok && s != "" {
			cols = splitCols(s)
		}
		for _, r := range t.rows {
			for _, c := range cols {
				if Equal(r.vals[norm(c)], Arg(args, 0)) {
					return r, nil
				}
			}
		}
		return Undefined{}, nil
	case "найтистроки":
		filter, ok := Arg(args, 0).(*Structure)
		if !ok {
			return nil, fmt.Errorf("Неверный тип параметра: ожидается Структура")
		}
		out := NewArray()
		for _, r := range t.rows {
			match := true
			for _, k := range filter.order {
				if !Equal(r.value(k), filter.vals[k]) {
					match = false
					break
				}
			}
			if match {
				out.Elems = append(out.Elems, r)
			}
		}
		return out, nil
	case "сортировать":
		s, _ := Arg(args, 0).(string)
		return nil, t.sort(s)
	case "свернуть":
		g, _ := Arg(args, 0).(string)
		sum, _ := Arg(args, 1).(string)
		return nil, t.groupBy(splitCols(g), splitCols(sum))
	case "выгрузитьколонку":
		c := ToString(Arg(args, 0))
		if !t.hasCol(c) {
			return nil, errNoField(c)
		}
		out := NewArray()
		for _, r := range t.rows {
			out.Elems = append(out.Elems, r.value(norm(c)))
		}
		return out, nil
	case "загрузитьколонку":
		arr, ok := Arg(args, 0).(*Array)
		c := ToString(Arg(args, 1))
		if !ok || !t.hasCol(c) {
			return nil, fmt.Errorf("Неверные параметры ЗагрузитьКолонку")
		}
		for i, v := range arr.Elems {
			if i >= len(t.rows) {
				break
			}
			t.rows[i].vals[norm(c)] = v
		}
		return nil, nil
	case "итог":
		c := ToString(Arg(args, 0))
		sum := NewInt(0)
		for _, r := range t.rows {
			if n, ok := r.value(norm(c)).(Number); ok {
				sum = sum.Add(n)
			}
		}
		return sum, nil
	case "скопировать":
		c := NewValueTable()
		cols := t.cols
		if s, ok := Arg(args, 1).(string); ok && s != "" {
			cols = splitCols(s)
		}
		c.cols = append(c.cols, cols...)
		src := t.rows
		if arr, ok := Arg(args, 0).(*Array); ok {
			src = nil
			for _, e := range arr.Elems {
				if r, ok := e.(*ValueTableRow); ok {
					src = append(src, r)
				}
			}
		}
		for _, r := range src {
			nr := c.AddRow()
			for _, col := range cols {
				nr.vals[norm(col)] = r.value(norm(col))
			}
		}
		return c, nil
	}
	return nil, errNoMethod(name)
}

func (t *ValueTable) sort(spec string) error {
	type key struct {
		col  string
		desc bool
	}
	var keys []key
	for _, part := range splitCols(spec) {
		f := strings.Fields(part)
		k := key{col: norm(f[0])}
		if len(f) > 1 && (norm(f[1]) == "убыв" || norm(f[1]) == "desc") {
			k.desc = true
		}
		if !t.hasCol(f[0]) {
			return errNoField(f[0])
		}
		keys = append(keys, k)
	}
	var sortErr error
	sort.SliceStable(t.rows, func(i, j int) bool {
		for _, k := range keys {
			c, err := Compare(t.rows[i].value(k.col), t.rows[j].value(k.col))
			if err != nil {
				sortErr = err
				return false
			}
			if c != 0 {
				return (c < 0) != k.desc
			}
		}
		return false
	})
	return sortErr
}

func (t *ValueTable) groupBy(group, sum []string) error {
	for _, c := range append(append([]string{}, group...), sum...) {
		if !t.hasCol(c) {
			return errNoField(c)
		}
	}
	var out []*ValueTableRow
	index := map[string]*ValueTableRow{}
	for _, r := range t.rows {
		var kb strings.Builder
		for _, g := range group {
			kb.WriteString(mapKey(r.value(norm(g))))
			kb.WriteByte(0)
		}
		acc, ok := index[kb.String()]
		if !ok {
			acc = &ValueTableRow{table: t, vals: map[string]Value{}}
			for _, g := range group {
				acc.vals[norm(g)] = r.value(norm(g))
			}
			for _, s := range sum {
				acc.vals[norm(s)] = NewInt(0)
			}
			index[kb.String()] = acc
			out = append(out, acc)
		}
		for _, s := range sum {
			if n, ok := r.value(norm(s)).(Number); ok {
				acc.vals[norm(s)] = acc.vals[norm(s)].(Number).Add(n)
			}
		}
	}
	t.cols = append(append([]string{}, group...), sum...)
	t.rows = out
	return nil
}

// ValueTableRow is СтрокаТаблицыЗначений: columns are its properties.
type ValueTableRow struct {
	table *ValueTable
	vals  map[string]Value
}

func (*ValueTableRow) TypeName() string { return "СтрокаТаблицыЗначений" }

func (r *ValueTableRow) value(col string) Value {
	if v, ok := r.vals[norm(col)]; ok {
		return v
	}
	return Undefined{}
}

// Value returns the value of a column.
func (r *ValueTableRow) Value(col string) Value { return r.value(col) }

func (r *ValueTableRow) Get(name string) (Value, bool) {
	if !r.table.hasCol(name) {
		return nil, false
	}
	return r.value(name), true
}

func (r *ValueTableRow) Set(name string, v Value) error {
	if !r.table.hasCol(name) {
		return errNoField(name)
	}
	r.vals[norm(name)] = v
	return nil
}

func (r *ValueTableRow) colName(k Value) (string, error) {
	switch x := k.(type) {
	case string:
		return x, nil
	case Number:
		i := int(x.Int64())
		if i < 0 || i >= len(r.table.cols) {
			return "", errIndex()
		}
		return r.table.cols[i], nil
	}
	return "", fmt.Errorf("Неверный тип индекса")
}

func (r *ValueTableRow) GetIndex(k Value) (Value, error) {
	c, err := r.colName(k)
	if err != nil {
		return nil, err
	}
	if v, ok := r.Get(c); ok {
		return v, nil
	}
	return nil, errNoField(c)
}

func (r *ValueTableRow) SetIndex(k, v Value) error {
	c, err := r.colName(k)
	if err != nil {
		return err
	}
	return r.Set(c, v)
}

func (r *ValueTableRow) CallMethod(_ *Interp, name string, _ []*Ref) (Value, error) {
	if norm(name) == "владелец" || norm(name) == "owner" {
		return r.table, nil
	}
	return nil, errNoMethod(name)
}

// tableColumns is КоллекцияКолонокТаблицыЗначений.
type tableColumns struct{ t *ValueTable }

func (*tableColumns) TypeName() string {
	return "КоллекцияКолонокТаблицыЗначений"
}

func (c *tableColumns) Items() []Value {
	out := make([]Value, len(c.t.cols))
	for i, n := range c.t.cols {
		out[i] = &tableColumn{name: n}
	}
	return out
}

var columnsMethods = methods("Добавить", "Add", "Количество", "Count", "Найти", "Find", "Удалить", "Delete")

func (c *tableColumns) CallMethod(_ *Interp, name string, args []*Ref) (Value, error) {
	switch columnsMethods.id(name) {
	case "добавить":
		n := ToString(Arg(args, 0))
		if err := c.t.AddColumn(n); err != nil {
			return nil, err
		}
		return &tableColumn{name: n}, nil
	case "количество":
		return NewInt(int64(len(c.t.cols))), nil
	case "найти":
		n := ToString(Arg(args, 0))
		for _, col := range c.t.cols {
			if norm(col) == norm(n) {
				return &tableColumn{name: col}, nil
			}
		}
		return Undefined{}, nil
	case "удалить":
		n := ToString(Arg(args, 0))
		if col, ok := Arg(args, 0).(*tableColumn); ok {
			n = col.name
		}
		for i, col := range c.t.cols {
			if norm(col) == norm(n) {
				c.t.cols = append(c.t.cols[:i], c.t.cols[i+1:]...)
				for _, r := range c.t.rows {
					delete(r.vals, norm(n))
				}
				return nil, nil
			}
		}
		return nil, errNoField(n)
	}
	return nil, errNoMethod(name)
}

type tableColumn struct{ name string }

func (*tableColumn) TypeName() string { return "КолонкаТаблицыЗначений" }

func (c *tableColumn) Get(name string) (Value, bool) {
	if norm(name) == "имя" || norm(name) == "name" {
		return c.name, true
	}
	return nil, false
}

// Compare orders two values of one type; values of different types cannot
// be compared, as in the platform.
func Compare(a, b Value) (int, error) {
	switch x := a.(type) {
	case Number:
		if y, ok := b.(Number); ok {
			return x.Cmp(y), nil
		}
	case string:
		if y, ok := b.(string); ok {
			return strings.Compare(x, y), nil
		}
	case time.Time:
		if y, ok := b.(time.Time); ok {
			return x.Compare(y), nil
		}
	case bool:
		if y, ok := b.(bool); ok {
			switch {
			case x == y:
				return 0, nil
			case !x:
				return -1, nil
			}
			return 1, nil
		}
	case Undefined, nil:
		switch b.(type) {
		case Undefined, nil:
			return 0, nil
		}
	}
	return 0, fmt.Errorf("Операция сравнения на больше-меньше допустима только для значений совпадающих примитивных типов (Булево, Число, Строка, Дата)")
}
