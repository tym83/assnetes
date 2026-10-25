package bsl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Builtin is a function of the global context. Arguments come as
// references so that a function can write output parameters.
type Builtin func(it *Interp, args []*Ref) (Value, error)

// Constructor makes a value for «Новый Тип(...)».
type Constructor func(it *Interp, args []Value) (Value, error)

// Interp is one global context: its functions, types and properties. It is
// not safe for use by several goroutines at once; make one per goroutine.
type Interp struct {
	funcs   map[string]Builtin
	types   map[string]Constructor
	globals map[string]*Ref

	// Message receives what Сообщить() says. Without it messages go to Out.
	Message func(text string)
	Out     io.Writer

	// MaxSteps bounds the statements one call may execute; 0 means no bound.
	MaxSteps int64
	steps    int64
	ctx      context.Context

	// Now is the clock of ТекущаяДата(); tests and calendars replace it.
	Now func() time.Time
	cur *frame // the frame calling a builtin

	// Env reads the environment for ПолучитьПеременнуюСреды(); nil denies it.
	Env func(name string) (string, bool)
}

// New makes a global context with the standard library of the language.
func New() *Interp {
	it := &Interp{
		funcs:   map[string]Builtin{},
		types:   map[string]Constructor{},
		globals: map[string]*Ref{},
		Out:     os.Stdout,
		ctx:     context.Background(),
		Now:     time.Now,
	}
	registerStdlib(it)
	return it
}

// Define adds or replaces a global function under all of its names.
func (it *Interp) Define(fn Builtin, names ...string) {
	for _, n := range names {
		it.funcs[norm(n)] = fn
	}
}

// DefineType adds a type for «Новый» under all of its names.
func (it *Interp) DefineType(c Constructor, names ...string) {
	for _, n := range names {
		it.types[norm(n)] = c
	}
}

// SetGlobal sets a property of the global context, such as a host object.
func (it *Interp) SetGlobal(v Value, names ...string) {
	r := &Ref{V: v}
	for _, n := range names {
		it.globals[norm(n)] = r
	}
}

// WithContext sets the context that cancels execution.
func (it *Interp) WithContext(ctx context.Context) { it.ctx = ctx }

// NewObject makes a value of a type by name, as «Новый» does.
func (it *Interp) NewObject(typeName string, args ...Value) (Value, error) {
	c, ok := it.types[norm(typeName)]
	if !ok {
		return nil, fmt.Errorf("Тип не определен (%s)", typeName)
	}
	return c(it, args)
}

// Exception is a runtime error of a script.
type Exception struct {
	Description string
	Module      string
	Line        int
	Cause       *Exception
}

func (e *Exception) Error() string {
	if e.Module == "" && e.Line == 0 {
		return e.Description
	}
	return fmt.Sprintf("{%s(%d)}: %s", e.Module, e.Line, e.Description)
}

func asException(err error, mod string, line int) *Exception {
	var ex *Exception
	if errors.As(err, &ex) {
		return ex
	}
	return &Exception{Description: err.Error(), Module: mod, Line: line}
}

// Script is a module loaded into a context: its variables live here.
type Script struct {
	it   *Interp
	mod  *Module
	vars map[string]*Ref
}

// Module returns the compiled module of the script.
func (s *Script) Module() *Module { return s.mod }

// Load runs the body of a module and returns the loaded script.
func (it *Interp) Load(mod *Module) (*Script, error) {
	s := &Script{it: it, mod: mod, vars: map[string]*Ref{}}
	for _, d := range mod.Vars {
		for _, n := range d.Names {
			s.vars[norm(n)] = &Ref{V: Undefined{}}
		}
	}
	it.steps = 0
	f := &frame{s: s, locals: s.vars}
	if err := f.block(mod.Body); err != nil {
		return nil, s.finish(err)
	}
	return s, nil
}

// Run compiles and runs source as one module.
func (it *Interp) Run(name, src string) (*Script, error) {
	mod, err := Compile(name, src)
	if err != nil {
		return nil, err
	}
	return it.Load(mod)
}

// Var returns a module variable.
func (s *Script) Var(name string) (Value, bool) {
	r, ok := s.vars[norm(name)]
	if !ok {
		return nil, false
	}
	return r.V, true
}

// Call calls a method of the module with values.
func (s *Script) Call(name string, args ...Value) (Value, error) {
	refs := make([]*Ref, len(args))
	for i, a := range args {
		refs[i] = &Ref{V: a}
	}
	return s.CallRefs(name, refs)
}

// CallRefs calls a method of the module; the method may change the values
// of the references, the way event handlers set Отказ.
func (s *Script) CallRefs(name string, args []*Ref) (Value, error) {
	m := s.mod.Method(name)
	if m == nil {
		return nil, &Exception{Description: fmt.Sprintf("Метод объекта не обнаружен (%s)", name), Module: s.mod.Name}
	}
	s.it.steps = 0
	v, err := s.invoke(m, args)
	return v, s.finish(err)
}

func (s *Script) finish(err error) error {
	if err == nil {
		return nil
	}
	switch err.(type) {
	case *returnSignal:
		return nil
	case loopSignal:
		return &Exception{Description: "Прервать или Продолжить вне цикла", Module: s.mod.Name}
	}
	return err
}

func (s *Script) invoke(m *Method, args []*Ref) (Value, error) {
	if len(args) > len(m.Params) {
		return nil, &Exception{Description: "Слишком много фактических параметров", Module: s.mod.Name, Line: m.Line}
	}
	f := &frame{s: s, m: m, locals: map[string]*Ref{}}
	for i, p := range m.Params {
		var r *Ref
		if i < len(args) && args[i] != nil {
			r = args[i]
			if p.ByVal {
				r = &Ref{V: r.V}
			}
		}
		if r == nil {
			r = &Ref{V: Undefined{}}
			if p.Default != nil {
				v, err := f.eval(p.Default)
				if err != nil {
					return nil, err
				}
				r.V = v
			}
		}
		f.locals[norm(p.Name)] = r
	}
	for _, d := range m.Vars {
		for _, n := range d.Names {
			f.locals[norm(n)] = &Ref{V: Undefined{}}
		}
	}
	err := f.block(m.Body)
	if rs, ok := err.(*returnSignal); ok {
		if !m.IsFunction {
			return Undefined{}, nil
		}
		return rs.v, nil
	}
	if err != nil {
		return nil, err
	}
	return Undefined{}, nil
}

type frame struct {
	s      *Script
	m      *Method // nil for the module body
	locals map[string]*Ref
	exc    *Exception // being handled in Исключение
}

type returnSignal struct{ v Value }

func (*returnSignal) Error() string { return "Возврат" }

type loopSignal string

func (l loopSignal) Error() string { return string(l) }

const (
	sigBreak    loopSignal = "Прервать"
	sigContinue loopSignal = "Продолжить"
)

func (f *frame) fail(line int, format string, args ...any) error {
	return &Exception{Description: fmt.Sprintf(format, args...), Module: f.s.mod.Name, Line: line}
}

func (f *frame) wrap(err error, line int) error {
	if err == nil {
		return nil
	}
	switch err.(type) {
	case *returnSignal, loopSignal, *Exception:
		return err
	}
	return asException(err, f.s.mod.Name, line)
}

func (f *frame) block(stmts []Stmt) error {
	for _, s := range stmts {
		if err := f.stmt(s); err != nil {
			return err
		}
	}
	return nil
}

func (f *frame) tick(line int) error {
	it := f.s.it
	it.steps++
	if it.MaxSteps > 0 && it.steps > it.MaxSteps {
		return f.fail(line, "Превышено допустимое время выполнения: модуль выполнил больше %d операторов", it.MaxSteps)
	}
	if it.steps%1024 == 0 {
		if err := it.ctx.Err(); err != nil {
			return f.fail(line, "Выполнение прервано: %v", err)
		}
	}
	return nil
}

func (f *frame) stmt(s Stmt) error {
	if err := f.tick(s.stmtLine()); err != nil {
		return err
	}
	switch s := s.(type) {
	case *AssignStmt:
		v, err := f.eval(s.Value)
		if err != nil {
			return err
		}
		return f.wrap(f.assign(s.Target, v), s.Line)
	case *ExprStmt:
		_, err := f.call(s.X.(*Call), false)
		return err
	case *IfStmt:
		for i, c := range s.Conds {
			b, err := f.cond(c)
			if err != nil {
				return err
			}
			if b {
				return f.block(s.Blocks[i])
			}
		}
		return f.block(s.Else)
	case *WhileStmt:
		for {
			b, err := f.cond(s.Cond)
			if err != nil || !b {
				return err
			}
			if err := f.loopBody(s.Body); err != nil {
				if err == sigBreak {
					return nil
				}
				return err
			}
		}
	case *ForStmt:
		from, err := f.number(s.From)
		if err != nil {
			return err
		}
		to, err := f.number(s.To)
		if err != nil {
			return err
		}
		one := NewInt(1)
		for i := from; i.Cmp(to) <= 0; {
			if err := f.setVar(s.Var, i); err != nil {
				return err
			}
			if err := f.loopBody(s.Body); err != nil {
				if err == sigBreak {
					return nil
				}
				return err
			}
			cur, _ := f.lookup(s.Var)
			n, ok := cur.(Number)
			if !ok {
				return f.fail(s.Line, "Переменная цикла должна оставаться числом")
			}
			i = n.Add(one)
		}
		return nil
	case *ForEachStmt:
		in, err := f.eval(s.In)
		if err != nil {
			return err
		}
		iter, ok := in.(Iterable)
		if !ok {
			return f.fail(s.Line, "Итератор для значения не определен (%s)", TypeOf(in))
		}
		for _, item := range iter.Items() {
			if err := f.setVar(s.Var, item); err != nil {
				return err
			}
			if err := f.loopBody(s.Body); err != nil {
				if err == sigBreak {
					return nil
				}
				return err
			}
		}
		return nil
	case *TryStmt:
		err := f.block(s.Body)
		if err == nil {
			return nil
		}
		switch err.(type) {
		case *returnSignal, loopSignal:
			return err
		}
		ex := asException(err, f.s.mod.Name, s.Line)
		prev := f.exc
		f.exc = ex
		err = f.block(s.Except)
		f.exc = prev
		return err
	case *RaiseStmt:
		if s.X == nil {
			if f.exc == nil {
				return f.fail(s.Line, "ВызватьИсключение без параметра допустимо только в блоке Исключение")
			}
			return f.exc
		}
		v, err := f.eval(s.X)
		if err != nil {
			return err
		}
		ex := &Exception{Description: ToString(v), Module: f.s.mod.Name, Line: s.Line}
		if f.exc != nil {
			ex.Cause = f.exc
		}
		return ex
	case *ReturnStmt:
		if s.X == nil {
			if f.m != nil && f.m.IsFunction {
				return &returnSignal{v: Undefined{}}
			}
			return &returnSignal{}
		}
		if f.m == nil || !f.m.IsFunction {
			return f.fail(s.Line, "Процедура не может возвращать значение")
		}
		v, err := f.eval(s.X)
		if err != nil {
			return err
		}
		return &returnSignal{v: v}
	case *BreakStmt:
		return sigBreak
	case *ContinueStmt:
		return sigContinue
	}
	return f.fail(s.stmtLine(), "неизвестный оператор")
}

func (f *frame) loopBody(body []Stmt) error {
	if err := f.tick(0); err != nil { // an empty loop must still be bounded
		return err
	}
	err := f.block(body)
	if err == sigContinue {
		return nil
	}
	return err
}

func (f *frame) cond(e Expr) (bool, error) {
	v, err := f.eval(e)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, f.fail(e.exprLine(), "Преобразование значения к типу Булево не может быть выполнено")
	}
	return b, nil
}

func (f *frame) number(e Expr) (Number, error) {
	v, err := f.eval(e)
	if err != nil {
		return Number{}, err
	}
	n, ok := v.(Number)
	if !ok {
		return Number{}, f.fail(e.exprLine(), "Преобразование значения к типу Число не может быть выполнено")
	}
	return n, nil
}

func (f *frame) ref(name string) (*Ref, bool) {
	k := norm(name)
	if r, ok := f.locals[k]; ok {
		return r, true
	}
	if r, ok := f.s.vars[k]; ok {
		return r, true
	}
	if r, ok := f.s.it.globals[k]; ok {
		return r, true
	}
	return nil, false
}

func (f *frame) lookup(name string) (Value, bool) {
	r, ok := f.ref(name)
	if !ok {
		return nil, false
	}
	return r.V, true
}

func (f *frame) setVar(name string, v Value) error {
	k := norm(name)
	if r, ok := f.locals[k]; ok {
		r.V = v
		return nil
	}
	if r, ok := f.s.vars[k]; ok {
		r.V = v
		return nil
	}
	if _, ok := f.s.it.globals[k]; ok {
		return fmt.Errorf("Свойство глобального контекста недоступно для записи (%s)", name)
	}
	f.locals[k] = &Ref{V: v}
	return nil
}

func (f *frame) assign(target Expr, v Value) error {
	switch t := target.(type) {
	case *Ident:
		return f.setVar(t.Name, v)
	case *Member:
		obj, err := f.eval(t.X)
		if err != nil {
			return err
		}
		s, ok := obj.(Setter)
		if !ok {
			return fmt.Errorf("Поле объекта не обнаружено (%s)", t.Name)
		}
		return s.Set(t.Name, v)
	case *Index:
		obj, err := f.eval(t.X)
		if err != nil {
			return err
		}
		k, err := f.eval(t.Key)
		if err != nil {
			return err
		}
		ix, ok := obj.(Indexer)
		if !ok {
			return fmt.Errorf("Получение элемента по индексу для значения не определено (%s)", TypeOf(obj))
		}
		return ix.SetIndex(k, v)
	}
	return fmt.Errorf("недопустимая левая часть присваивания")
}

func (f *frame) eval(e Expr) (Value, error) {
	switch e := e.(type) {
	case *Literal:
		return e.Value, nil
	case *Ident:
		if v, ok := f.lookup(e.Name); ok {
			return v, nil
		}
		return nil, f.fail(e.Line, "Переменная не определена (%s)", e.Name)
	case *Member:
		obj, err := f.eval(e.X)
		if err != nil {
			return nil, err
		}
		if g, ok := obj.(Getter); ok {
			if v, ok := g.Get(e.Name); ok {
				return v, nil
			}
		}
		return nil, f.fail(e.Line, "Поле объекта не обнаружено (%s)", e.Name)
	case *Index:
		obj, err := f.eval(e.X)
		if err != nil {
			return nil, err
		}
		k, err := f.eval(e.Key)
		if err != nil {
			return nil, err
		}
		ix, ok := obj.(Indexer)
		if !ok {
			return nil, f.fail(e.Line, "Получение элемента по индексу для значения не определено (%s)", TypeOf(obj))
		}
		v, err := ix.GetIndex(k)
		return v, f.wrap(err, e.Line)
	case *Call:
		return f.call(e, true)
	case *NewExpr:
		return f.newObject(e)
	case *Unary:
		v, err := f.eval(e.X)
		if err != nil {
			return nil, err
		}
		switch e.Op {
		case "не":
			b, ok := v.(bool)
			if !ok {
				return nil, f.fail(e.Line, "Преобразование значения к типу Булево не может быть выполнено")
			}
			return !b, nil
		case "-":
			n, ok := v.(Number)
			if !ok {
				return nil, f.fail(e.Line, "Преобразование значения к типу Число не может быть выполнено")
			}
			return n.Neg(), nil
		default:
			n, ok := v.(Number)
			if !ok {
				return nil, f.fail(e.Line, "Преобразование значения к типу Число не может быть выполнено")
			}
			return n, nil
		}
	case *Binary:
		return f.binary(e)
	case *Ternary:
		b, err := f.cond(e.Cond)
		if err != nil {
			return nil, err
		}
		if b {
			return f.eval(e.Then)
		}
		return f.eval(e.Else)
	}
	return nil, fmt.Errorf("неизвестное выражение")
}

func (f *frame) binary(e *Binary) (Value, error) {
	if e.Op == "и" || e.Op == "или" {
		l, err := f.cond(e.L)
		if err != nil {
			return nil, err
		}
		if e.Op == "и" && !l || e.Op == "или" && l {
			return l, nil
		}
		return f.cond(e.R)
	}
	l, err := f.eval(e.L)
	if err != nil {
		return nil, err
	}
	r, err := f.eval(e.R)
	if err != nil {
		return nil, err
	}
	v, err := Arith(e.Op, l, r)
	return v, f.wrap(err, e.Line)
}

// Arith applies a binary operator of the language to two values.
func Arith(op string, l, r Value) (Value, error) {
	switch op {
	case "=":
		return Equal(l, r), nil
	case "<>":
		return !Equal(l, r), nil
	case "<", ">", "<=", ">=":
		c, err := Compare(l, r)
		if err != nil {
			return nil, err
		}
		switch op {
		case "<":
			return c < 0, nil
		case ">":
			return c > 0, nil
		case "<=":
			return c <= 0, nil
		}
		return c >= 0, nil
	}
	switch x := l.(type) {
	case string:
		if op == "+" {
			return x + ToString(r), nil
		}
	case time.Time:
		switch y := r.(type) {
		case Number:
			secs := y.Int64()
			if op == "+" {
				return x.Add(time.Duration(secs) * time.Second), nil
			}
			if op == "-" {
				return x.Add(-time.Duration(secs) * time.Second), nil
			}
		case time.Time:
			if op == "-" {
				return NewInt(int64(x.Sub(y) / time.Second)), nil
			}
		}
	case Number:
		y, ok := r.(Number)
		if !ok {
			if s, isStr := r.(string); isStr {
				n, err := ParseNumber(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), " ", ""))
				if err != nil {
					return nil, fmt.Errorf("Преобразование значения к типу Число не может быть выполнено")
				}
				y, ok = n, true
			}
		}
		if ok {
			switch op {
			case "+":
				return x.Add(y), nil
			case "-":
				return x.Sub(y), nil
			case "*":
				return x.Mul(y), nil
			case "/":
				return x.Div(y)
			case "%":
				return x.Mod(y)
			}
		}
	case bool:
		return nil, fmt.Errorf("Преобразование значения к типу Число не может быть выполнено")
	}
	return nil, fmt.Errorf("Арифметическая операция над значениями типов %s и %s не определена", TypeOf(l), TypeOf(r))
}

// args evaluates call arguments into references: a bare variable is passed
// by reference, anything else by value.
func (f *frame) args(exprs []Expr) ([]*Ref, error) {
	out := make([]*Ref, len(exprs))
	for i, a := range exprs {
		if a == nil {
			continue
		}
		if id, ok := a.(*Ident); ok {
			k := norm(id.Name)
			if r, ok := f.locals[k]; ok {
				out[i] = r
				continue
			}
			if r, ok := f.s.vars[k]; ok {
				out[i] = r
				continue
			}
			if _, ok := f.s.it.globals[k]; !ok {
				// an output parameter declares the variable: Свойство("Ключ", Значение)
				r := &Ref{V: Undefined{}}
				f.locals[k] = r
				out[i] = r
				continue
			}
		}
		v, err := f.eval(a)
		if err != nil {
			return nil, err
		}
		out[i] = &Ref{V: v}
	}
	return out, nil
}

func (f *frame) call(c *Call, wantValue bool) (Value, error) {
	switch fn := c.Fn.(type) {
	case *Ident:
		if m := f.s.mod.Method(fn.Name); m != nil {
			if wantValue && !m.IsFunction {
				return nil, f.fail(c.Line, "Использование процедуры в качестве функции (%s)", fn.Name)
			}
			args, err := f.args(c.Args)
			if err != nil {
				return nil, err
			}
			v, err := f.s.invoke(m, args)
			return v, f.wrap(err, c.Line)
		}
		if b, ok := f.s.it.funcs[norm(fn.Name)]; ok {
			args, err := f.args(c.Args)
			if err != nil {
				return nil, err
			}
			v, err := b(f.withFrame(), args)
			if err != nil {
				return nil, f.wrap(err, c.Line)
			}
			if v == nil {
				v = Undefined{}
			}
			return v, nil
		}
		return nil, f.fail(c.Line, "Процедура или функция с указанным именем не определена (%s)", fn.Name)
	case *Member:
		obj, err := f.eval(fn.X)
		if err != nil {
			return nil, err
		}
		cl, ok := obj.(Caller)
		if !ok {
			return nil, f.fail(c.Line, "Метод объекта не обнаружен (%s)", fn.Name)
		}
		args, err := f.args(c.Args)
		if err != nil {
			return nil, err
		}
		v, err := cl.CallMethod(f.withFrame(), fn.Name, args)
		if err != nil {
			return nil, f.wrap(err, c.Line)
		}
		if v == nil {
			v = Undefined{}
		}
		return v, nil
	}
	return nil, f.fail(c.Line, "ожидается имя процедуры или функции")
}

// withFrame lets builtins such as ИнформацияОбОшибке() see the frame.
func (f *frame) withFrame() *Interp {
	f.s.it.cur = f
	return f.s.it
}

func (f *frame) newObject(e *NewExpr) (Value, error) {
	name := e.Type
	var argv []Value
	if e.TypeExpr != nil {
		tv, err := f.eval(e.TypeExpr)
		if err != nil {
			return nil, err
		}
		switch t := tv.(type) {
		case string:
			name = t
		case TypeValue:
			name = t.Name
		default:
			return nil, f.fail(e.Line, "Неверное имя типа")
		}
		if len(e.Args) > 0 && e.Args[0] != nil {
			pv, err := f.eval(e.Args[0])
			if err != nil {
				return nil, err
			}
			if a, ok := pv.(*Array); ok {
				argv = a.Elems
			}
		}
	} else {
		for _, a := range e.Args {
			if a == nil {
				argv = append(argv, Undefined{})
				continue
			}
			v, err := f.eval(a)
			if err != nil {
				return nil, err
			}
			argv = append(argv, v)
		}
	}
	c, ok := f.s.it.types[norm(name)]
	if !ok {
		return nil, f.fail(e.Line, "Тип не определен (%s)", name)
	}
	f.withFrame()
	v, err := c(f.s.it, argv)
	return v, f.wrap(err, e.Line)
}

// Context returns the context execution runs under.
func (it *Interp) Context() context.Context { return it.ctx }
