// Package zapros is the query language of 1C over the objects of a cluster:
//
//	ВЫБРАТЬ Поды.Подразделение, КОЛИЧЕСТВО(*) КАК Всего
//	ИЗ Кластер.Поды КАК Поды
//	ГДЕ Поды.Фаза = "Running"
//	СГРУППИРОВАТЬ ПО Поды.Подразделение
//	УПОРЯДОЧИТЬ ПО Всего УБЫВ
//
// Sources are kinds of the cluster, «Кластер.<вид>», and accumulation
// registers computed from them, «РегистрНакопления.РесурсыУзлов.Остатки».
package zapros

import (
	"fmt"
	"strings"
	"unicode"
)

type tok struct {
	kind string // id, num, str, param, op, kw, eof
	text string
	kw   string // canonical keyword
}

var keywords = map[string]string{}

func init() {
	for canon, names := range map[string][]string{
		"select": {"выбрать", "select"}, "distinct": {"различные", "distinct"}, "top": {"первые", "top"},
		"as": {"как", "as"}, "from": {"из", "from"}, "where": {"где", "where"},
		"and": {"и", "and"}, "or": {"или", "or"}, "not": {"не", "not"},
		"group": {"сгруппировать", "group"}, "by": {"по", "by", "on"}, "having": {"имеющие", "having"},
		"order": {"упорядочить", "order"}, "desc": {"убыв", "desc"}, "asc": {"возр", "asc"},
		"left": {"левое", "left"}, "inner": {"внутреннее", "inner"}, "join": {"соединение", "join"},
		"like": {"подобно", "like"}, "in": {"в", "in"}, "between": {"между", "between"},
		"is": {"есть", "is"}, "null": {"null"}, "true": {"истина", "true"}, "false": {"ложь", "false"},
		"case": {"выбор", "case"}, "when": {"когда", "when"}, "then": {"тогда", "then"},
		"else": {"иначе", "else"}, "end": {"конец", "end"}, "undefined": {"неопределено", "undefined"},
	} {
		for _, n := range names {
			keywords[n] = canon
		}
	}
}

func lex(src string) ([]tok, error) {
	var out []tok
	rs := []rune(src)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '/' && i+1 < len(rs) && rs[i+1] == '/':
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
		case r == '"':
			var b strings.Builder
			i++
			for {
				if i >= len(rs) {
					return nil, fmt.Errorf("незавершённая строка")
				}
				if rs[i] == '"' {
					if i+1 < len(rs) && rs[i+1] == '"' {
						b.WriteRune('"')
						i += 2
						continue
					}
					i++
					break
				}
				b.WriteRune(rs[i])
				i++
			}
			out = append(out, tok{kind: "str", text: b.String()})
		case r == '&':
			j := i + 1
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			out = append(out, tok{kind: "param", text: string(rs[i+1 : j])})
			i = j
		case unicode.IsDigit(r):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.' && j+1 < len(rs) && unicode.IsDigit(rs[j+1])) {
				j++
			}
			out = append(out, tok{kind: "num", text: string(rs[i:j])})
			i = j
		case unicode.IsLetter(r) || r == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			w := string(rs[i:j])
			if k, ok := keywords[strings.ToLower(w)]; ok {
				out = append(out, tok{kind: "kw", text: w, kw: k})
			} else {
				out = append(out, tok{kind: "id", text: w})
			}
			i = j
		default:
			two := ""
			if i+1 < len(rs) {
				two = string(rs[i : i+2])
			}
			if two == "<>" || two == "<=" || two == ">=" {
				out = append(out, tok{kind: "op", text: two})
				i += 2
				continue
			}
			if strings.ContainsRune("+-*/=<>(),.;", r) {
				out = append(out, tok{kind: "op", text: string(r)})
				i++
				continue
			}
			return nil, fmt.Errorf("недопустимый символ «%c» в тексте запроса", r)
		}
	}
	return append(out, tok{kind: "eof"}), nil
}

// Query is a parsed query.
type Query struct {
	Distinct bool
	Top      int
	Fields   []Field
	From     Source
	Joins    []Join
	Where    Expr
	GroupBy  []Expr
	Having   Expr
	OrderBy  []Order
}

type Field struct {
	X     Expr // nil for «*»
	Alias string
}

type Source struct {
	Path  []string // Кластер.Поды, РегистрНакопления.РесурсыУзлов.Остатки
	Args  []Expr
	Alias string
}

type Join struct {
	Left bool
	Src  Source
	On   Expr
}

type Order struct {
	X    Expr
	Desc bool
}

type Expr interface{}

type (
	Lit   struct{ V any }
	Param struct{ Name string }
	Path  struct{ Parts []string }
	Bin   struct {
		Op   string
		L, R Expr
	}
	Not  struct{ X Expr }
	Neg  struct{ X Expr }
	Like struct {
		X, Pattern Expr
		Not        bool
	}
	In struct {
		X    Expr
		List []Expr
		Not  bool
	}
	Between struct {
		X, Lo, Hi Expr
		Not       bool
	}
	IsNull struct {
		X   Expr
		Not bool
	}
	Case struct {
		Whens []Expr
		Thens []Expr
		Else  Expr
	}
	Call struct {
		Name     string // folded
		Args     []Expr
		Star     bool
		Distinct bool
	}
)

type parser struct {
	t   []tok
	pos int
}

func (p *parser) cur() tok { return p.t[p.pos] }
func (p *parser) next() tok {
	t := p.t[p.pos]
	if p.pos < len(p.t)-1 {
		p.pos++
	}
	return t
}
func (p *parser) isKw(k string) bool  { return p.cur().kind == "kw" && p.cur().kw == k }
func (p *parser) isOp(op string) bool { return p.cur().kind == "op" && p.cur().text == op }
func (p *parser) expectKw(k, word string) error {
	if !p.isKw(k) {
		return fmt.Errorf("ожидается %s, а не «%s»", word, p.cur().text)
	}
	p.next()
	return nil
}
func (p *parser) expectOp(op string) error {
	if !p.isOp(op) {
		return fmt.Errorf("ожидается «%s», а не «%s»", op, p.cur().text)
	}
	p.next()
	return nil
}

// Parse reads the text of a query.
func Parse(text string) (*Query, error) {
	t, err := lex(text)
	if err != nil {
		return nil, err
	}
	p := &parser{t: t}
	q, err := p.query()
	if err != nil {
		return nil, fmt.Errorf("Ошибка в тексте запроса: %w", err)
	}
	for p.isOp(";") {
		p.next()
	}
	if p.cur().kind != "eof" {
		return nil, fmt.Errorf("Ошибка в тексте запроса: лишнее «%s»; пакеты запросов не поддерживаются", p.cur().text)
	}
	return q, nil
}

func (p *parser) query() (*Query, error) {
	q := &Query{}
	if err := p.expectKw("select", "ВЫБРАТЬ"); err != nil {
		return nil, err
	}
	for {
		switch {
		case p.isKw("distinct"):
			p.next()
			q.Distinct = true
			continue
		case p.isKw("top"):
			p.next()
			n := p.next()
			if n.kind != "num" {
				return nil, fmt.Errorf("после ПЕРВЫЕ ожидается число")
			}
			fmt.Sscan(n.text, &q.Top)
			continue
		}
		break
	}
	for {
		var f Field
		if p.isOp("*") {
			p.next()
		} else {
			x, err := p.expr()
			if err != nil {
				return nil, err
			}
			f.X = x
			if p.isKw("as") {
				p.next()
				if p.cur().kind != "id" {
					return nil, fmt.Errorf("после КАК ожидается псевдоним")
				}
				f.Alias = p.next().text
			} else if pt, ok := x.(*Path); ok {
				f.Alias = pt.Parts[len(pt.Parts)-1]
			}
		}
		q.Fields = append(q.Fields, f)
		if !p.isOp(",") {
			break
		}
		p.next()
	}
	if err := p.expectKw("from", "ИЗ"); err != nil {
		return nil, err
	}
	src, err := p.source()
	if err != nil {
		return nil, err
	}
	q.From = src
	for p.isKw("left") || p.isKw("inner") || p.isKw("join") {
		j := Join{Left: p.isKw("left")}
		if !p.isKw("join") {
			p.next()
		}
		if err := p.expectKw("join", "СОЕДИНЕНИЕ"); err != nil {
			return nil, err
		}
		if j.Src, err = p.source(); err != nil {
			return nil, err
		}
		if err := p.expectKw("by", "ПО"); err != nil {
			return nil, err
		}
		if j.On, err = p.expr(); err != nil {
			return nil, err
		}
		q.Joins = append(q.Joins, j)
	}
	if p.isKw("where") {
		p.next()
		if q.Where, err = p.expr(); err != nil {
			return nil, err
		}
	}
	if p.isKw("group") {
		p.next()
		if err := p.expectKw("by", "ПО"); err != nil {
			return nil, err
		}
		for {
			x, err := p.expr()
			if err != nil {
				return nil, err
			}
			q.GroupBy = append(q.GroupBy, x)
			if !p.isOp(",") {
				break
			}
			p.next()
		}
	}
	if p.isKw("having") {
		p.next()
		if q.Having, err = p.expr(); err != nil {
			return nil, err
		}
	}
	if p.isKw("order") {
		p.next()
		if err := p.expectKw("by", "ПО"); err != nil {
			return nil, err
		}
		for {
			x, err := p.expr()
			if err != nil {
				return nil, err
			}
			o := Order{X: x}
			if p.isKw("desc") {
				p.next()
				o.Desc = true
			} else if p.isKw("asc") {
				p.next()
			}
			q.OrderBy = append(q.OrderBy, o)
			if !p.isOp(",") {
				break
			}
			p.next()
		}
	}
	return q, nil
}

func (p *parser) source() (Source, error) {
	var s Source
	for {
		if p.cur().kind != "id" {
			return s, fmt.Errorf("ожидается источник данных: Кластер.<Вид> или РегистрНакопления.<Имя>.<Таблица>")
		}
		s.Path = append(s.Path, p.next().text)
		if !p.isOp(".") {
			break
		}
		p.next()
	}
	if p.isOp("(") {
		p.next()
		for !p.isOp(")") {
			if p.isOp(",") {
				p.next()
				s.Args = append(s.Args, nil)
				continue
			}
			x, err := p.expr()
			if err != nil {
				return s, err
			}
			s.Args = append(s.Args, x)
			if p.isOp(",") {
				p.next()
			}
		}
		p.next()
	}
	if p.isKw("as") {
		p.next()
		if p.cur().kind != "id" {
			return s, fmt.Errorf("после КАК ожидается псевдоним")
		}
		s.Alias = p.next().text
	} else {
		s.Alias = s.Path[len(s.Path)-1]
	}
	return s, nil
}

func (p *parser) expr() (Expr, error) { return p.or() }

func (p *parser) or() (Expr, error) {
	l, err := p.and()
	for err == nil && p.isKw("or") {
		p.next()
		var r Expr
		if r, err = p.and(); err == nil {
			l = &Bin{Op: "или", L: l, R: r}
		}
	}
	return l, err
}

func (p *parser) and() (Expr, error) {
	l, err := p.not()
	for err == nil && p.isKw("and") {
		p.next()
		var r Expr
		if r, err = p.not(); err == nil {
			l = &Bin{Op: "и", L: l, R: r}
		}
	}
	return l, err
}

func (p *parser) not() (Expr, error) {
	if p.isKw("not") {
		p.next()
		x, err := p.not()
		return &Not{X: x}, err
	}
	return p.cmp()
}

func (p *parser) cmp() (Expr, error) {
	l, err := p.add()
	if err != nil {
		return nil, err
	}
	neg := false
	if p.isKw("not") && (p.t[p.pos+1].kw == "like" || p.t[p.pos+1].kw == "in" || p.t[p.pos+1].kw == "between") {
		p.next()
		neg = true
	}
	switch {
	case p.cur().kind == "op" && strings.Contains("= <> < > <= >=", p.cur().text) && p.cur().text != "":
		op := p.next().text
		r, err := p.add()
		return &Bin{Op: op, L: l, R: r}, err
	case p.isKw("like"):
		p.next()
		r, err := p.add()
		return &Like{X: l, Pattern: r, Not: neg}, err
	case p.isKw("in"):
		p.next()
		in := &In{X: l, Not: neg}
		if p.cur().kind == "param" {
			in.List = []Expr{&Param{Name: p.next().text}}
			return in, nil
		}
		if err := p.expectOp("("); err != nil {
			return nil, err
		}
		for !p.isOp(")") {
			x, err := p.expr()
			if err != nil {
				return nil, err
			}
			in.List = append(in.List, x)
			if p.isOp(",") {
				p.next()
			}
		}
		p.next()
		return in, nil
	case p.isKw("between"):
		p.next()
		lo, err := p.add()
		if err != nil {
			return nil, err
		}
		if err := p.expectKw("and", "И"); err != nil {
			return nil, err
		}
		hi, err := p.add()
		return &Between{X: l, Lo: lo, Hi: hi, Not: neg}, err
	case p.isKw("is"):
		p.next()
		n := &IsNull{X: l}
		if p.isKw("not") {
			p.next()
			n.Not = true
		}
		return n, p.expectKw("null", "NULL")
	}
	return l, nil
}

func (p *parser) add() (Expr, error) {
	l, err := p.mul()
	for err == nil && (p.isOp("+") || p.isOp("-")) {
		op := p.next().text
		var r Expr
		if r, err = p.mul(); err == nil {
			l = &Bin{Op: op, L: l, R: r}
		}
	}
	return l, err
}

func (p *parser) mul() (Expr, error) {
	l, err := p.unary()
	for err == nil && (p.isOp("*") || p.isOp("/")) {
		op := p.next().text
		var r Expr
		if r, err = p.unary(); err == nil {
			l = &Bin{Op: op, L: l, R: r}
		}
	}
	return l, err
}

func (p *parser) unary() (Expr, error) {
	if p.isOp("-") {
		p.next()
		x, err := p.unary()
		return &Neg{X: x}, err
	}
	return p.primary()
}

var aggregates = map[string]string{
	"количество": "count", "count": "count", "сумма": "sum", "sum": "sum",
	"минимум": "min", "min": "min", "максимум": "max", "max": "max", "среднее": "avg", "avg": "avg",
}

func (p *parser) primary() (Expr, error) {
	t := p.cur()
	switch t.kind {
	case "num":
		p.next()
		return &Lit{V: t.text}, nil
	case "str":
		p.next()
		return &Lit{V: t.text}, nil
	case "param":
		p.next()
		return &Param{Name: t.text}, nil
	case "kw":
		switch t.kw {
		case "true":
			p.next()
			return &Lit{V: true}, nil
		case "false":
			p.next()
			return &Lit{V: false}, nil
		case "null", "undefined":
			p.next()
			return &Lit{V: nil}, nil
		case "case":
			return p.caseExpr()
		}
	case "op":
		if t.text == "(" {
			p.next()
			x, err := p.expr()
			if err != nil {
				return nil, err
			}
			return x, p.expectOp(")")
		}
	case "id":
		p.next()
		if p.isOp("(") {
			p.next()
			c := &Call{Name: strings.ToLower(t.text)}
			if a, ok := aggregates[c.Name]; ok {
				c.Name = a
			}
			if p.isKw("distinct") {
				p.next()
				c.Distinct = true
			}
			if p.isOp("*") {
				p.next()
				c.Star = true
			}
			for !p.isOp(")") {
				x, err := p.expr()
				if err != nil {
					return nil, err
				}
				c.Args = append(c.Args, x)
				if p.isOp(",") {
					p.next()
				} else if !p.isOp(")") {
					return nil, fmt.Errorf("ожидается «)»")
				}
			}
			p.next()
			return c, nil
		}
		path := &Path{Parts: []string{t.text}}
		for p.isOp(".") {
			p.next()
			if p.cur().kind != "id" && p.cur().kind != "kw" {
				return nil, fmt.Errorf("ожидается имя поля после точки")
			}
			path.Parts = append(path.Parts, p.next().text)
		}
		return path, nil
	}
	return nil, fmt.Errorf("ожидается выражение, а не «%s»", t.text)
}

func (p *parser) caseExpr() (Expr, error) {
	p.next()
	c := &Case{}
	for p.isKw("when") {
		p.next()
		w, err := p.expr()
		if err != nil {
			return nil, err
		}
		if err := p.expectKw("then", "ТОГДА"); err != nil {
			return nil, err
		}
		th, err := p.expr()
		if err != nil {
			return nil, err
		}
		c.Whens = append(c.Whens, w)
		c.Thens = append(c.Thens, th)
	}
	if p.isKw("else") {
		p.next()
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		c.Else = e
	}
	return c, p.expectKw("end", "КОНЕЦ")
}
