package bsl

import (
	"fmt"
	"strings"
)

// Compile parses the source of a module. The module name only appears in
// error messages.
func Compile(name, src string) (*Module, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, withModule(err, name)
	}
	mod := &Module{Name: name, methods: map[string]*Method{}}
	toks, err = preprocess(toks, mod)
	if err != nil {
		return nil, withModule(err, name)
	}
	p := &parser{toks: toks, mod: mod}
	if err := p.parseModule(); err != nil {
		return nil, withModule(err, name)
	}
	return mod, nil
}

func withModule(err error, name string) error {
	if se, ok := err.(*SyntaxError); ok {
		se.Module = name
	}
	return err
}

// preprocess drops regions and other directives and resolves #Если by the
// symbols of the environment scripts run in: a server, on Linux.
func preprocess(toks []token, mod *Module) ([]token, error) {
	type frame struct{ active, taken, parentActive bool }
	var stack []frame
	active := true
	var out []token
	for _, t := range toks {
		if t.kind != tokPreproc {
			if active {
				out = append(out, t)
			}
			continue
		}
		word, rest, _ := strings.Cut(t.text, " ")
		rest = strings.TrimSpace(rest)
		switch norm(word) {
		case "если", "if":
			c, err := evalPreprocCond(rest, t.line)
			if err != nil {
				return nil, err
			}
			stack = append(stack, frame{active: active && c, taken: c, parentActive: active})
			active = active && c
		case "иначеесли", "elsif":
			if len(stack) == 0 {
				return nil, &SyntaxError{Line: t.line, Message: "#ИначеЕсли без #Если"}
			}
			f := &stack[len(stack)-1]
			c, err := evalPreprocCond(rest, t.line)
			if err != nil {
				return nil, err
			}
			f.active = f.parentActive && !f.taken && c
			f.taken = f.taken || c
			active = f.active
		case "иначе", "else":
			if len(stack) == 0 {
				return nil, &SyntaxError{Line: t.line, Message: "#Иначе без #Если"}
			}
			f := &stack[len(stack)-1]
			f.active = f.parentActive && !f.taken
			f.taken = true
			active = f.active
		case "конецесли", "endif":
			if len(stack) == 0 {
				return nil, &SyntaxError{Line: t.line, Message: "#КонецЕсли без #Если"}
			}
			active = stack[len(stack)-1].parentActive
			stack = stack[:len(stack)-1]
		case "использовать", "use":
			if active {
				mod.Uses = append(mod.Uses, strings.Trim(rest, `"`))
			}
		case "удаление", "delete":
			stack = append(stack, frame{active: false, taken: true, parentActive: active})
			active = false
		case "конецудаления", "enddelete", "вставка", "insert", "конецвставки", "endinsert":
			if norm(word) == "конецудаления" || norm(word) == "enddelete" {
				if len(stack) == 0 {
					return nil, &SyntaxError{Line: t.line, Message: "#КонецУдаления без #Удаление"}
				}
				active = stack[len(stack)-1].parentActive
				stack = stack[:len(stack)-1]
			}
		default: // #Область, #КонецОбласти, #native and the like
		}
	}
	if len(stack) > 0 {
		return nil, &SyntaxError{Line: toks[len(toks)-1].line, Message: "ожидается #КонецЕсли"}
	}
	return out, nil
}

var preprocSymbols = map[string]bool{
	"сервер": true, "server": true, "насервере": true, "atserver": true,
	"внешнеесоединение": true, "externalconnection": true, "linux": true,
}

func evalPreprocCond(s string, line int) (bool, error) {
	s = strings.TrimSpace(s)
	low := norm(s)
	if strings.HasSuffix(low, "тогда") {
		s = s[:len(s)-len("тогда")]
	} else if strings.HasSuffix(low, "then") {
		s = s[:len(s)-len("then")]
	}
	words := strings.Fields(strings.NewReplacer("(", " ( ", ")", " ) ").Replace(s))
	pos := 0
	var or, and, not func() (bool, error)
	or = func() (bool, error) {
		v, err := and()
		for err == nil && pos < len(words) && (norm(words[pos]) == "или" || norm(words[pos]) == "or") {
			pos++
			var r bool
			r, err = and()
			v = v || r
		}
		return v, err
	}
	and = func() (bool, error) {
		v, err := not()
		for err == nil && pos < len(words) && (norm(words[pos]) == "и" || norm(words[pos]) == "and") {
			pos++
			var r bool
			r, err = not()
			v = v && r
		}
		return v, err
	}
	not = func() (bool, error) {
		if pos >= len(words) {
			return false, &SyntaxError{Line: line, Message: "ожидается условие препроцессора"}
		}
		w := words[pos]
		pos++
		switch norm(w) {
		case "не", "not":
			v, err := not()
			return !v, err
		case "(":
			v, err := or()
			if pos >= len(words) || words[pos] != ")" {
				return false, &SyntaxError{Line: line, Message: "ожидается символ ')'"}
			}
			pos++
			return v, err
		}
		return preprocSymbols[norm(w)], nil
	}
	v, err := or()
	if err == nil && pos != len(words) {
		err = &SyntaxError{Line: line, Message: "неверное условие препроцессора"}
	}
	return v, err
}

type parser struct {
	toks []token
	pos  int
	mod  *Module
}

func (p *parser) cur() token  { return p.toks[p.pos] }
func (p *parser) peek() token { return p.toks[min(p.pos+1, len(p.toks)-1)] }
func (p *parser) advance() token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) isKw(k keyword) bool { return p.cur().kind == tokKeyword && p.cur().kw == k }
func (p *parser) isOp(op string) bool { return p.cur().kind == tokOp && p.cur().text == op }

func (p *parser) errorf(format string, args ...any) error {
	return &SyntaxError{Line: p.cur().line, Message: fmt.Sprintf(format, args...)}
}

func (p *parser) expectOp(op string) error {
	if !p.isOp(op) {
		return p.errorf("ожидается символ '%s'", op)
	}
	p.advance()
	return nil
}

func (p *parser) expectKw(k keyword, word string) error {
	if !p.isKw(k) {
		return p.errorf("ожидается ключевое слово '%s'", word)
	}
	p.advance()
	return nil
}

func (p *parser) ident() (string, error) {
	if p.cur().kind != tokIdent {
		return "", p.errorf("ожидается идентификатор")
	}
	return p.advance().text, nil
}

func (p *parser) parseModule() error {
	for p.cur().kind != tokEOF {
		anns, err := p.parseAnnotations()
		if err != nil {
			return err
		}
		switch {
		case p.isKw(kwVar):
			d, err := p.parseVar(anns)
			if err != nil {
				return err
			}
			p.mod.Vars = append(p.mod.Vars, d)
		case p.isKw(kwProcedure) || p.isKw(kwFunction):
			m, err := p.parseMethod(anns)
			if err != nil {
				return err
			}
			if p.mod.methods[norm(m.Name)] != nil {
				return &SyntaxError{Line: m.Line, Message: fmt.Sprintf("процедура или функция с указанным именем уже определена (%s)", m.Name)}
			}
			p.mod.methods[norm(m.Name)] = m
			p.mod.Methods = append(p.mod.Methods, m)
		default:
			if len(anns) > 0 {
				return p.errorf("аннотация должна стоять перед объявлением")
			}
			s, err := p.parseStmt()
			if err != nil {
				return err
			}
			p.mod.Body = append(p.mod.Body, s)
			if err := p.endOfStmt(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseAnnotations() ([]Annotation, error) {
	var anns []Annotation
	for p.cur().kind == tokAnnotation {
		a := Annotation{Name: p.advance().text}
		if p.isOp("(") {
			p.advance()
			for !p.isOp(")") {
				var ap AnnotationParam
				if p.cur().kind == tokIdent && p.peek().kind == tokOp && p.peek().text == "=" {
					ap.Name = p.advance().text
					p.advance()
					v, err := p.annotationValue()
					if err != nil {
						return nil, err
					}
					ap.Value = v
				} else if p.cur().kind == tokIdent {
					ap.Name = p.advance().text
					ap.Value = Undefined{}
				} else {
					v, err := p.annotationValue()
					if err != nil {
						return nil, err
					}
					ap.Value = v
				}
				a.Params = append(a.Params, ap)
				if p.isOp(",") {
					p.advance()
				} else if !p.isOp(")") {
					return nil, p.errorf("ожидается символ ')'")
				}
			}
			p.advance()
		}
		anns = append(anns, a)
	}
	return anns, nil
}

func (p *parser) annotationValue() (Value, error) {
	e, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if lit, ok := e.(*Literal); ok {
		return lit.Value, nil
	}
	if u, ok := e.(*Unary); ok && u.Op == "-" {
		if lit, ok := u.X.(*Literal); ok {
			if n, ok := lit.Value.(Number); ok {
				return n.Neg(), nil
			}
		}
	}
	return nil, p.errorf("значение параметра аннотации должно быть литералом")
}

func (p *parser) parseVar(anns []Annotation) (*VarDecl, error) {
	d := &VarDecl{Line: p.advance().line, Annotations: anns}
	for {
		n, err := p.ident()
		if err != nil {
			return nil, err
		}
		d.Names = append(d.Names, n)
		if p.isKw(kwExport) {
			p.advance()
			d.Export = true
		}
		if !p.isOp(",") {
			break
		}
		p.advance()
	}
	return d, p.expectOp(";")
}

func (p *parser) parseMethod(anns []Annotation) (*Method, error) {
	isFunc := p.isKw(kwFunction)
	m := &Method{IsFunction: isFunc, Annotations: anns, Line: p.advance().line}
	var err error
	if m.Name, err = p.ident(); err != nil {
		return nil, err
	}
	if err := p.expectOp("("); err != nil {
		return nil, err
	}
	for !p.isOp(")") {
		var prm Param
		for p.cur().kind == tokAnnotation { // annotations of parameters are ignored
			if _, err := p.parseAnnotations(); err != nil {
				return nil, err
			}
		}
		if p.isKw(kwVal) {
			p.advance()
			prm.ByVal = true
		}
		if prm.Name, err = p.ident(); err != nil {
			return nil, err
		}
		if p.isOp("=") {
			p.advance()
			if prm.Default, err = p.parseUnary(); err != nil {
				return nil, err
			}
		}
		m.Params = append(m.Params, prm)
		if p.isOp(",") {
			p.advance()
		} else if !p.isOp(")") {
			return nil, p.errorf("ожидается символ ')'")
		}
	}
	p.advance()
	if p.isKw(kwExport) {
		p.advance()
		m.Export = true
	}
	for p.isKw(kwVar) {
		d, err := p.parseVar(nil)
		if err != nil {
			return nil, err
		}
		m.Vars = append(m.Vars, d)
	}
	end, word := kwEndProcedure, "КонецПроцедуры"
	if isFunc {
		end, word = kwEndFunction, "КонецФункции"
	}
	if m.Body, err = p.parseBlock(end); err != nil {
		return nil, err
	}
	return m, p.expectKw(end, word)
}

// parseBlock reads statements until one of the given keywords.
func (p *parser) parseBlock(ends ...keyword) ([]Stmt, error) {
	var out []Stmt
	for {
		if p.cur().kind == tokEOF {
			return nil, p.errorf("неожиданный конец модуля")
		}
		if p.isOp(";") {
			p.advance()
			continue
		}
		if p.cur().kind == tokKeyword {
			for _, e := range ends {
				if p.cur().kw == e {
					return out, nil
				}
			}
		}
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
		if err := p.endOfStmt(ends...); err != nil {
			return nil, err
		}
	}
}

func (p *parser) endOfStmt(ends ...keyword) error {
	if p.isOp(";") {
		p.advance()
		return nil
	}
	if p.cur().kind == tokEOF {
		return nil
	}
	if p.cur().kind == tokKeyword {
		for _, e := range ends {
			if p.cur().kw == e {
				return nil
			}
		}
	}
	return p.errorf("ожидается символ ';'")
}

func (p *parser) parseStmt() (Stmt, error) {
	t := p.cur()
	line := t.line
	if t.kind == tokKeyword {
		switch t.kw {
		case kwIf:
			return p.parseIf()
		case kwFor:
			return p.parseFor()
		case kwWhile:
			p.advance()
			cond, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectKw(kwDo, "Цикл"); err != nil {
				return nil, err
			}
			body, err := p.parseBlock(kwEndDo)
			if err != nil {
				return nil, err
			}
			return &WhileStmt{Cond: cond, Body: body, Line: line}, p.expectKw(kwEndDo, "КонецЦикла")
		case kwTry:
			p.advance()
			body, err := p.parseBlock(kwExcept)
			if err != nil {
				return nil, err
			}
			p.advance()
			exc, err := p.parseBlock(kwEndTry)
			if err != nil {
				return nil, err
			}
			return &TryStmt{Body: body, Except: exc, Line: line}, p.expectKw(kwEndTry, "КонецПопытки")
		case kwRaise:
			p.advance()
			if p.isOp(";") || p.cur().kind == tokKeyword && p.cur().kw != kwNot && p.cur().kw != kwNew &&
				p.cur().kw != kwTrue && p.cur().kw != kwFalse && p.cur().kw != kwUndefined && p.cur().kw != kwNull {
				return &RaiseStmt{Line: line}, nil
			}
			if p.isOp("(") {
				args, err := p.parseArgs()
				if err != nil {
					return nil, err
				}
				if len(args) == 0 || args[0] == nil {
					return nil, p.errorf("ожидается выражение")
				}
				var x Expr = args[0]
				if len(args) == 1 {
					// a parenthesised expression may go on: ВызватьИсключение ("а") + Б;
					x, err = p.continueBinary(x)
					if err != nil {
						return nil, err
					}
				}
				return &RaiseStmt{X: x, Line: line}, nil
			}
			x, err := p.parseExpr()
			return &RaiseStmt{X: x, Line: line}, err
		case kwReturn:
			p.advance()
			if p.isOp(";") || p.cur().kind == tokEOF || p.cur().kind == tokKeyword &&
				(p.cur().kw == kwEndProcedure || p.cur().kw == kwEndFunction || p.cur().kw == kwEndIf ||
					p.cur().kw == kwElse || p.cur().kw == kwElsIf || p.cur().kw == kwEndDo ||
					p.cur().kw == kwEndTry || p.cur().kw == kwExcept) {
				return &ReturnStmt{Line: line}, nil
			}
			x, err := p.parseExpr()
			return &ReturnStmt{X: x, Line: line}, err
		case kwBreak:
			p.advance()
			return &BreakStmt{Line: line}, nil
		case kwContinue:
			p.advance()
			return &ContinueStmt{Line: line}, nil
		case kwGoto:
			return nil, p.errorf("оператор Перейти не поддерживается: у нас не 7.7")
		case kwExecute:
			p.advance()
			args, err := p.parseArgs()
			if err != nil {
				return nil, err
			}
			return &ExprStmt{X: &Call{Fn: &Ident{Name: "Выполнить", Line: line}, Args: args, Line: line}, Line: line}, nil
		}
	}
	if p.isOp("~") {
		return nil, p.errorf("метки и Перейти не поддерживаются")
	}
	x, err := p.parsePostfix()
	if err != nil {
		return nil, err
	}
	if p.isOp("=") {
		switch x.(type) {
		case *Ident, *Member, *Index:
		default:
			return nil, p.errorf("левая часть присваивания должна быть переменной, свойством или элементом")
		}
		p.advance()
		v, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return &AssignStmt{Target: x, Value: v, Line: line}, nil
	}
	if _, ok := x.(*Call); !ok {
		return nil, &SyntaxError{Line: line, Message: "ожидается оператор"}
	}
	return &ExprStmt{X: x, Line: line}, nil
}

func (p *parser) parseIf() (Stmt, error) {
	s := &IfStmt{Line: p.advance().line}
	for {
		cond, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if err := p.expectKw(kwThen, "Тогда"); err != nil {
			return nil, err
		}
		block, err := p.parseBlock(kwElsIf, kwElse, kwEndIf)
		if err != nil {
			return nil, err
		}
		s.Conds = append(s.Conds, cond)
		s.Blocks = append(s.Blocks, block)
		if p.isKw(kwElsIf) {
			p.advance()
			continue
		}
		break
	}
	if p.isKw(kwElse) {
		p.advance()
		block, err := p.parseBlock(kwEndIf)
		if err != nil {
			return nil, err
		}
		s.Else = block
	}
	return s, p.expectKw(kwEndIf, "КонецЕсли")
}

func (p *parser) parseFor() (Stmt, error) {
	line := p.advance().line
	if p.isKw(kwEach) {
		p.advance()
		name, err := p.ident()
		if err != nil {
			return nil, err
		}
		if err := p.expectKw(kwIn, "Из"); err != nil {
			return nil, err
		}
		in, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if err := p.expectKw(kwDo, "Цикл"); err != nil {
			return nil, err
		}
		body, err := p.parseBlock(kwEndDo)
		if err != nil {
			return nil, err
		}
		return &ForEachStmt{Var: name, In: in, Body: body, Line: line}, p.expectKw(kwEndDo, "КонецЦикла")
	}
	name, err := p.ident()
	if err != nil {
		return nil, err
	}
	if err := p.expectOp("="); err != nil {
		return nil, err
	}
	from, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.expectKw(kwTo, "По"); err != nil {
		return nil, err
	}
	to, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.expectKw(kwDo, "Цикл"); err != nil {
		return nil, err
	}
	body, err := p.parseBlock(kwEndDo)
	if err != nil {
		return nil, err
	}
	return &ForStmt{Var: name, From: from, To: to, Body: body, Line: line}, p.expectKw(kwEndDo, "КонецЦикла")
}

// Expressions, from the loosest binding: ИЛИ, И, НЕ, comparison, + -, * / %,
// unary minus, postfix.

func (p *parser) parseExpr() (Expr, error) { return p.parseOr() }

func (p *parser) parseOr() (Expr, error) {
	l, err := p.parseAnd()
	for err == nil && p.isKw(kwOr) {
		line := p.advance().line
		var r Expr
		if r, err = p.parseAnd(); err == nil {
			l = &Binary{Op: "или", L: l, R: r, Line: line}
		}
	}
	return l, err
}

func (p *parser) parseAnd() (Expr, error) {
	l, err := p.parseNot()
	for err == nil && p.isKw(kwAnd) {
		line := p.advance().line
		var r Expr
		if r, err = p.parseNot(); err == nil {
			l = &Binary{Op: "и", L: l, R: r, Line: line}
		}
	}
	return l, err
}

func (p *parser) parseNot() (Expr, error) {
	if p.isKw(kwNot) {
		line := p.advance().line
		x, err := p.parseNot()
		return &Unary{Op: "не", X: x, Line: line}, err
	}
	return p.parseCompare()
}

func (p *parser) parseCompare() (Expr, error) {
	l, err := p.parseAdd()
	for err == nil && p.cur().kind == tokOp {
		op := p.cur().text
		if op != "=" && op != "<>" && op != "<" && op != ">" && op != "<=" && op != ">=" {
			break
		}
		line := p.advance().line
		var r Expr
		if r, err = p.parseAdd(); err == nil {
			l = &Binary{Op: op, L: l, R: r, Line: line}
		}
	}
	return l, err
}

func (p *parser) parseAdd() (Expr, error) {
	l, err := p.parseMul()
	for err == nil && (p.isOp("+") || p.isOp("-")) {
		t := p.advance()
		var r Expr
		if r, err = p.parseMul(); err == nil {
			l = &Binary{Op: t.text, L: l, R: r, Line: t.line}
		}
	}
	return l, err
}

func (p *parser) parseMul() (Expr, error) {
	l, err := p.parseUnary()
	for err == nil && (p.isOp("*") || p.isOp("/") || p.isOp("%")) {
		t := p.advance()
		var r Expr
		if r, err = p.parseUnary(); err == nil {
			l = &Binary{Op: t.text, L: l, R: r, Line: t.line}
		}
	}
	return l, err
}

func (p *parser) parseUnary() (Expr, error) {
	if p.isOp("-") || p.isOp("+") {
		t := p.advance()
		x, err := p.parseUnary()
		return &Unary{Op: t.text, X: x, Line: t.line}, err
	}
	return p.parsePostfix()
}

// continueBinary goes on parsing an expression whose first operand is x.
func (p *parser) continueBinary(x Expr) (Expr, error) {
	for p.cur().kind == tokOp && strings.Contains("+-*/%", p.cur().text) && len(p.cur().text) == 1 {
		t := p.advance()
		r, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		x = &Binary{Op: t.text, L: x, R: r, Line: t.line}
	}
	return x, nil
}

func (p *parser) parsePostfix() (Expr, error) {
	x, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.isOp("."):
			line := p.advance().line
			if p.cur().kind != tokIdent && p.cur().kind != tokKeyword {
				return nil, p.errorf("ожидается имя свойства или метода")
			}
			x = &Member{X: x, Name: p.advance().text, Line: line}
		case p.isOp("["):
			line := p.advance().line
			k, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp("]"); err != nil {
				return nil, err
			}
			x = &Index{X: x, Key: k, Line: line}
		case p.isOp("("):
			switch x.(type) {
			case *Ident, *Member:
			default:
				return x, nil
			}
			line := p.cur().line
			args, err := p.parseArgs()
			if err != nil {
				return nil, err
			}
			x = &Call{Fn: x, Args: args, Line: line}
		default:
			return x, nil
		}
	}
}

// parseArgs reads «(a, , b)»; an omitted argument is a nil element.
func (p *parser) parseArgs() ([]Expr, error) {
	if err := p.expectOp("("); err != nil {
		return nil, err
	}
	var args []Expr
	if p.isOp(")") {
		p.advance()
		return args, nil
	}
	for {
		if p.isOp(",") || p.isOp(")") {
			args = append(args, nil)
		} else {
			a, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, a)
		}
		if p.isOp(",") {
			p.advance()
			continue
		}
		return args, p.expectOp(")")
	}
}

func (p *parser) parsePrimary() (Expr, error) {
	t := p.cur()
	switch t.kind {
	case tokNumber:
		p.advance()
		n, err := ParseNumber(t.text)
		if err != nil {
			return nil, &SyntaxError{Line: t.line, Message: err.Error()}
		}
		return &Literal{Value: n, Line: t.line}, nil
	case tokString:
		p.advance()
		return &Literal{Value: t.text, Line: t.line}, nil
	case tokDate:
		p.advance()
		d, err := parseDateDigits(t.text)
		if err != nil {
			return nil, &SyntaxError{Line: t.line, Message: err.Error()}
		}
		return &Literal{Value: d, Line: t.line}, nil
	case tokIdent:
		p.advance()
		return &Ident{Name: t.text, Line: t.line}, nil
	case tokKeyword:
		switch t.kw {
		case kwTrue:
			p.advance()
			return &Literal{Value: true, Line: t.line}, nil
		case kwFalse:
			p.advance()
			return &Literal{Value: false, Line: t.line}, nil
		case kwUndefined:
			p.advance()
			return &Literal{Value: Undefined{}, Line: t.line}, nil
		case kwNull:
			p.advance()
			return &Literal{Value: Null{}, Line: t.line}, nil
		case kwNew:
			return p.parseNew()
		case kwExecute:
			p.advance()
			return &Ident{Name: t.text, Line: t.line}, nil
		}
	case tokOp:
		switch t.text {
		case "(":
			p.advance()
			x, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			return x, p.expectOp(")")
		case "?":
			p.advance()
			args, err := p.parseArgs()
			if err != nil {
				return nil, err
			}
			if len(args) != 3 || args[0] == nil || args[1] == nil || args[2] == nil {
				return nil, &SyntaxError{Line: t.line, Message: "ожидается ?(Условие, Значение1, Значение2)"}
			}
			return &Ternary{Cond: args[0], Then: args[1], Else: args[2], Line: t.line}, nil
		}
	}
	return nil, p.errorf("ожидается выражение")
}

func (p *parser) parseNew() (Expr, error) {
	line := p.advance().line
	if p.isOp("(") {
		args, err := p.parseArgs()
		if err != nil {
			return nil, err
		}
		if len(args) == 0 || args[0] == nil {
			return nil, p.errorf("ожидается имя типа")
		}
		return &NewExpr{TypeExpr: args[0], Args: args[1:], Line: line}, nil
	}
	name, err := p.ident()
	if err != nil {
		return nil, err
	}
	e := &NewExpr{Type: name, Line: line}
	if p.isOp("(") {
		if e.Args, err = p.parseArgs(); err != nil {
			return nil, err
		}
	}
	return e, nil
}
