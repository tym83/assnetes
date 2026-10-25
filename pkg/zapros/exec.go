package zapros

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tym83/assnetes/pkg/bsl"
)

// Data gives the rows of a kind of the cluster: objects in Russian form.
type Data interface {
	Table(kind string) ([]bsl.Value, error)
}

type row map[string]bsl.Value // alias (folded) → object

type ctx struct {
	q      *Query
	params map[string]bsl.Value
	only   string // the alias when there is one source
}

// Execute runs a query and returns its result as ТаблицаЗначений.
func Execute(q *Query, data Data, params map[string]bsl.Value) (*bsl.ValueTable, error) {
	c := &ctx{q: q, params: map[string]bsl.Value{}}
	for k, v := range params {
		c.params[strings.ToLower(k)] = v
	}
	if len(q.Joins) == 0 {
		c.only = strings.ToLower(q.From.Alias)
	}
	rows, err := c.load(q.From, data)
	if err != nil {
		return nil, err
	}
	for _, j := range q.Joins {
		right, err := c.load(j.Src, data)
		if err != nil {
			return nil, err
		}
		var joined []row
		for _, l := range rows {
			matched := false
			for _, r := range right {
				m := row{}
				for k, v := range l {
					m[k] = v
				}
				for k, v := range r {
					m[k] = v
				}
				ok, err := c.truth(j.On, m)
				if err != nil {
					return nil, err
				}
				if ok {
					joined = append(joined, m)
					matched = true
				}
			}
			if !matched && j.Left {
				joined = append(joined, l)
			}
		}
		rows = joined
	}
	if q.Where != nil {
		var kept []row
		for _, r := range rows {
			ok, err := c.truth(q.Where, r)
			if err != nil {
				return nil, err
			}
			if ok {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	cols := c.columns()
	out := bsl.NewValueTable()
	for _, col := range cols {
		if err := out.AddColumn(col); err != nil {
			return nil, fmt.Errorf("поле «%s» дважды: дайте псевдоним через КАК", col)
		}
	}
	type result struct {
		vals []bsl.Value
		src  []row
	}
	var results []result
	if len(q.GroupBy) > 0 || c.hasAggregates() {
		groups := map[string][]row{}
		var order []string
		for _, r := range rows {
			var key strings.Builder
			for _, g := range q.GroupBy {
				v, err := c.eval(g, r, nil)
				if err != nil {
					return nil, err
				}
				key.WriteString(fmt.Sprintf("%s|%s;", bsl.TypeOf(v), bsl.ToString(v)))
			}
			k := key.String()
			if _, ok := groups[k]; !ok {
				order = append(order, k)
			}
			groups[k] = append(groups[k], r)
		}
		if len(q.GroupBy) == 0 && len(order) == 0 {
			order = append(order, "")
			groups[""] = nil
		}
		for _, k := range order {
			g := groups[k]
			if q.Having != nil {
				ok, err := c.truthGroup(q.Having, g)
				if err != nil {
					return nil, err
				}
				if !ok {
					continue
				}
			}
			var vals []bsl.Value
			for _, f := range c.fields() {
				v, err := c.evalGroup(f.X, g)
				if err != nil {
					return nil, err
				}
				vals = append(vals, v)
			}
			results = append(results, result{vals: vals, src: g})
		}
	} else {
		for _, r := range rows {
			var vals []bsl.Value
			for _, f := range c.fields() {
				v, err := c.eval(f.X, r, nil)
				if err != nil {
					return nil, err
				}
				vals = append(vals, v)
			}
			results = append(results, result{vals: vals, src: []row{r}})
		}
	}
	if q.Distinct {
		seen := map[string]bool{}
		var kept []result
		for _, r := range results {
			var k strings.Builder
			for _, v := range r.vals {
				k.WriteString(bsl.TypeOf(v) + "|" + bsl.ToString(v) + ";")
			}
			if !seen[k.String()] {
				seen[k.String()] = true
				kept = append(kept, r)
			}
		}
		results = kept
	}
	if len(q.OrderBy) > 0 {
		keys := make([][]bsl.Value, len(results))
		for i, r := range results {
			for _, o := range q.OrderBy {
				v, err := c.orderValue(o.X, cols, r.vals, r.src)
				if err != nil {
					return nil, err
				}
				keys[i] = append(keys[i], v)
			}
		}
		idx := make([]int, len(results))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool {
			for k, o := range q.OrderBy {
				cmp := compare(keys[idx[a]][k], keys[idx[b]][k])
				if cmp != 0 {
					return (cmp < 0) != o.Desc
				}
			}
			return false
		})
		sorted := make([]result, len(results))
		for i, j := range idx {
			sorted[i] = results[j]
		}
		results = sorted
	}
	if q.Top > 0 && len(results) > q.Top {
		results = results[:q.Top]
	}
	for _, r := range results {
		tr := out.AddRow()
		for i, col := range cols {
			_ = tr.Set(col, r.vals[i])
		}
	}
	return out, nil
}

// compare orders values; NULL goes first, types that cannot be compared
// compare by their text.
func compare(a, b bsl.Value) int {
	_, an := a.(bsl.Undefined)
	_, bn := b.(bsl.Undefined)
	switch {
	case an && bn:
		return 0
	case an:
		return -1
	case bn:
		return 1
	}
	if c, err := bsl.Compare(a, b); err == nil {
		return c
	}
	return strings.Compare(bsl.ToString(a), bsl.ToString(b))
}

func (c *ctx) load(s Source, data Data) ([]row, error) {
	var objs []bsl.Value
	var err error
	switch {
	case len(s.Path) == 2 && strings.EqualFold(s.Path[0], "Кластер"):
		objs, err = data.Table(s.Path[1])
	case len(s.Path) == 3 && strings.EqualFold(s.Path[0], "РегистрНакопления"):
		objs, err = register(data, s.Path[1], s.Path[2])
	default:
		return nil, fmt.Errorf("Ошибка в тексте запроса: неизвестный источник «%s»", strings.Join(s.Path, "."))
	}
	if err != nil {
		return nil, err
	}
	alias := strings.ToLower(s.Alias)
	out := make([]row, len(objs))
	for i, o := range objs {
		out[i] = row{alias: o}
	}
	return out, nil
}

// fields expands «*» into the fields every object has.
func (c *ctx) fields() []Field {
	var out []Field
	for _, f := range c.q.Fields {
		if f.X != nil {
			out = append(out, f)
			continue
		}
		a := c.q.From.Alias
		for _, n := range []string{"Имя", "Подразделение", "Создан"} {
			out = append(out, Field{X: &Path{Parts: []string{a, n}}, Alias: n})
		}
	}
	return out
}

func (c *ctx) columns() []string {
	var cols []string
	for i, f := range c.fields() {
		name := f.Alias
		if name == "" {
			name = fmt.Sprintf("Поле%d", i+1)
		}
		cols = append(cols, name)
	}
	return cols
}

func (c *ctx) hasAggregates() bool {
	for _, f := range c.fields() {
		if hasAgg(f.X) {
			return true
		}
	}
	return c.q.Having != nil
}

func hasAgg(e Expr) bool {
	switch x := e.(type) {
	case *Call:
		if _, ok := aggFuncs[x.Name]; ok {
			return true
		}
		for _, a := range x.Args {
			if hasAgg(a) {
				return true
			}
		}
	case *Bin:
		return hasAgg(x.L) || hasAgg(x.R)
	case *Neg:
		return hasAgg(x.X)
	case *Case:
		for _, w := range append(append([]Expr{}, x.Whens...), x.Thens...) {
			if hasAgg(w) {
				return true
			}
		}
		return x.Else != nil && hasAgg(x.Else)
	}
	return false
}

var aggFuncs = map[string]bool{"count": true, "sum": true, "min": true, "max": true, "avg": true}

func (c *ctx) truth(e Expr, r row) (bool, error) {
	v, err := c.eval(e, r, nil)
	if err != nil {
		return false, err
	}
	b, _ := v.(bool)
	return b, nil
}

func (c *ctx) truthGroup(e Expr, g []row) (bool, error) {
	v, err := c.evalGroup(e, g)
	if err != nil {
		return false, err
	}
	b, _ := v.(bool)
	return b, nil
}

// evalGroup evaluates an expression over a group: aggregates over all of
// its rows, everything else over its first row.
func (c *ctx) evalGroup(e Expr, g []row) (bsl.Value, error) {
	var first row
	if len(g) > 0 {
		first = g[0]
	}
	return c.eval(e, first, g)
}

func (c *ctx) orderValue(e Expr, cols []string, vals []bsl.Value, src []row) (bsl.Value, error) {
	if p, ok := e.(*Path); ok && len(p.Parts) == 1 {
		for i, col := range cols {
			if strings.EqualFold(col, p.Parts[0]) {
				return vals[i], nil
			}
		}
	}
	if len(src) == 1 {
		return c.eval(e, src[0], nil)
	}
	return c.evalGroup(e, src)
}

func (c *ctx) eval(e Expr, r row, group []row) (bsl.Value, error) {
	switch x := e.(type) {
	case *Lit:
		switch v := x.V.(type) {
		case nil:
			return bsl.Undefined{}, nil
		case string:
			if n, err := bsl.ParseNumber(v); err == nil && isNumber(v) {
				return n, nil
			}
			return v, nil
		default:
			return v, nil
		}
	case *Param:
		v, ok := c.params[strings.ToLower(x.Name)]
		if !ok {
			return nil, fmt.Errorf("Не задано значение параметра «%s»", x.Name)
		}
		return v, nil
	case *Path:
		return c.path(x, r)
	case *Neg:
		v, err := c.eval(x.X, r, group)
		if err != nil {
			return nil, err
		}
		return bsl.Arith("-", bsl.NewInt(0), v)
	case *Not:
		v, err := c.eval(x.X, r, group)
		if err != nil {
			return nil, err
		}
		b, _ := v.(bool)
		return !b, nil
	case *Bin:
		l, err := c.eval(x.L, r, group)
		if err != nil {
			return nil, err
		}
		if x.Op == "и" || x.Op == "или" {
			lb, _ := l.(bool)
			if x.Op == "и" && !lb || x.Op == "или" && lb {
				return lb, nil
			}
			rv, err := c.eval(x.R, r, group)
			if err != nil {
				return nil, err
			}
			rb, _ := rv.(bool)
			return rb, nil
		}
		rv, err := c.eval(x.R, r, group)
		if err != nil {
			return nil, err
		}
		if isNull(l) || isNull(rv) {
			if x.Op == "=" || x.Op == "<>" || len(x.Op) <= 2 && strings.ContainsAny(x.Op, "<>") {
				return false, nil
			}
			return bsl.Undefined{}, nil
		}
		v, err := bsl.Arith(x.Op, l, rv)
		if err != nil && (x.Op == "<" || x.Op == ">" || x.Op == "<=" || x.Op == ">=") {
			return false, nil // values of different types are simply not ordered
		}
		return v, err
	case *Like:
		v, err := c.eval(x.X, r, group)
		if err != nil {
			return nil, err
		}
		p, err := c.eval(x.Pattern, r, group)
		if err != nil {
			return nil, err
		}
		ok := likeRegexp(bsl.ToString(p)).MatchString(bsl.ToString(v))
		return ok != x.Not, nil
	case *In:
		v, err := c.eval(x.X, r, group)
		if err != nil {
			return nil, err
		}
		var list []bsl.Value
		for _, item := range x.List {
			iv, err := c.eval(item, r, group)
			if err != nil {
				return nil, err
			}
			if it, ok := iv.(bsl.Iterable); ok {
				list = append(list, it.Items()...)
			} else {
				list = append(list, iv)
			}
		}
		found := false
		for _, item := range list {
			if bsl.Equal(v, item) {
				found = true
				break
			}
		}
		return found != x.Not, nil
	case *Between:
		v, err := c.eval(x.X, r, group)
		if err != nil {
			return nil, err
		}
		lo, err := c.eval(x.Lo, r, group)
		if err != nil {
			return nil, err
		}
		hi, err := c.eval(x.Hi, r, group)
		if err != nil {
			return nil, err
		}
		ok := compare(v, lo) >= 0 && compare(v, hi) <= 0
		return ok != x.Not, nil
	case *IsNull:
		v, err := c.eval(x.X, r, group)
		if err != nil {
			return nil, err
		}
		return isNull(v) != x.Not, nil
	case *Case:
		for i, w := range x.Whens {
			ok, err := c.eval(w, r, group)
			if err != nil {
				return nil, err
			}
			if b, _ := ok.(bool); b {
				return c.eval(x.Thens[i], r, group)
			}
		}
		if x.Else != nil {
			return c.eval(x.Else, r, group)
		}
		return bsl.Undefined{}, nil
	case *Call:
		return c.call(x, r, group)
	}
	return nil, fmt.Errorf("неизвестное выражение")
}

func isNumber(s string) bool {
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r == '.') {
			return false
		}
	}
	return s != ""
}

func isNull(v bsl.Value) bool {
	switch v.(type) {
	case nil, bsl.Undefined, bsl.Null:
		return true
	}
	return false
}

func likeRegexp(p string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("(?s)^")
	for _, r := range p {
		switch r {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func (c *ctx) call(x *Call, r row, group []row) (bsl.Value, error) {
	if aggFuncs[x.Name] {
		if group == nil {
			group = []row{r}
		}
		var vals []bsl.Value
		for _, g := range group {
			if g == nil {
				continue
			}
			if x.Star {
				vals = append(vals, true)
				continue
			}
			if len(x.Args) == 0 {
				return nil, fmt.Errorf("агрегатной функции нужен аргумент")
			}
			v, err := c.eval(x.Args[0], g, nil)
			if err != nil {
				return nil, err
			}
			if !isNull(v) {
				vals = append(vals, v)
			}
		}
		if x.Distinct {
			seen := map[string]bool{}
			var d []bsl.Value
			for _, v := range vals {
				k := bsl.TypeOf(v) + "|" + bsl.ToString(v)
				if !seen[k] {
					seen[k] = true
					d = append(d, v)
				}
			}
			vals = d
		}
		switch x.Name {
		case "count":
			return bsl.NewInt(int64(len(vals))), nil
		case "sum", "avg":
			sum := bsl.NewInt(0)
			for _, v := range vals {
				if n, ok := v.(bsl.Number); ok {
					sum = sum.Add(n)
				}
			}
			if x.Name == "avg" {
				if len(vals) == 0 {
					return bsl.Undefined{}, nil
				}
				return sum.Div(bsl.NewInt(int64(len(vals))))
			}
			return sum, nil
		case "min", "max":
			if len(vals) == 0 {
				return bsl.Undefined{}, nil
			}
			best := vals[0]
			for _, v := range vals[1:] {
				cmp := compare(v, best)
				if x.Name == "min" && cmp < 0 || x.Name == "max" && cmp > 0 {
					best = v
				}
			}
			return best, nil
		}
	}
	args := make([]bsl.Value, len(x.Args))
	for i, a := range x.Args {
		v, err := c.eval(a, r, group)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	switch x.Name {
	case "естьnull", "isnull":
		if len(args) == 2 && isNull(args[0]) {
			return args[1], nil
		}
		return args[0], nil
	case "подстрока", "substring":
		s := []rune(bsl.ToString(args[0]))
		from, _ := args[1].(bsl.Number)
		n, _ := args[2].(bsl.Number)
		start := max(int(from.Int64())-1, 0)
		end := min(start+int(n.Int64()), len(s))
		if start >= len(s) {
			return "", nil
		}
		return string(s[start:end]), nil
	case "количестворесурса", "quantity":
		return quantity(args[0]), nil
	}
	return nil, fmt.Errorf("Функция «%s» в языке запросов не поддерживается", x.Name)
}

// path resolves Алиас.Поле.Поле: a field is looked for in the object, then
// in its Метаданные, Спецификация and Состояние; Имя, Подразделение and
// Создан are short for fields of the metadata.
func (c *ctx) path(p *Path, r row) (bsl.Value, error) {
	parts := p.Parts
	obj, ok := r[strings.ToLower(parts[0])]
	if ok {
		parts = parts[1:]
	} else if c.only != "" {
		obj, ok = r[c.only]
	}
	if !ok {
		return nil, fmt.Errorf("Поле не найдено «%s»", strings.Join(p.Parts, "."))
	}
	if len(parts) == 0 {
		return obj, nil
	}
	v := obj
	for i, part := range parts {
		next, found := field(v, part)
		if !found && i == 0 {
			switch strings.ToLower(part) {
			case "создан":
				next, found = deep(v, "Метаданные", "ДатаСоздания")
			default:
				for _, sect := range []string{"Метаданные", "Спецификация", "Состояние"} {
					if next, found = deep(v, sect, part); found {
						break
					}
				}
			}
		}
		if !found {
			return bsl.Undefined{}, nil // NULL, as for a field nobody filled
		}
		v = next
	}
	return v, nil
}

func deep(v bsl.Value, names ...string) (bsl.Value, bool) {
	for _, n := range names {
		var ok bool
		if v, ok = field(v, n); !ok {
			return nil, false
		}
	}
	return v, true
}

func field(v bsl.Value, name string) (bsl.Value, bool) {
	switch o := v.(type) {
	case *bsl.Map:
		for _, k := range o.Keys() {
			if strings.EqualFold(bsl.ToString(k), name) {
				x, _ := o.Lookup(k)
				return x, true
			}
		}
	case *bsl.Structure:
		return o.Get(name)
	case interface {
		Entries() ([]string, []bsl.Value)
	}:
		keys, vals := o.Entries()
		for i, k := range keys {
			if strings.EqualFold(k, name) {
				return vals[i], true
			}
		}
	}
	return nil, false
}
