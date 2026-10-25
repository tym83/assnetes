package bsl

// Module is a compiled module: its variables, methods and body.
type Module struct {
	Name    string
	Uses    []string // #Использовать
	Vars    []*VarDecl
	Methods []*Method
	Body    []Stmt
	methods map[string]*Method
}

// Method finds a method of the module by name, in any case.
func (m *Module) Method(name string) *Method { return m.methods[norm(name)] }

type Annotation struct {
	Name   string
	Params []AnnotationParam
}

type AnnotationParam struct {
	Name  string // empty for a positional parameter
	Value Value
}

// Param returns a named parameter of the annotation, or the positional one
// at index pos when there is no such name; pos < 0 disables the fallback.
func (a *Annotation) Param(name string, pos int) (Value, bool) {
	for _, p := range a.Params {
		if norm(p.Name) == norm(name) {
			return p.Value, true
		}
	}
	n := 0
	for _, p := range a.Params {
		if p.Name == "" {
			if n == pos {
				return p.Value, true
			}
			n++
		}
	}
	return nil, false
}

type VarDecl struct {
	Names       []string
	Export      bool
	Annotations []Annotation
	Line        int
}

type Method struct {
	Name        string
	IsFunction  bool
	Export      bool
	Params      []Param
	Annotations []Annotation
	Vars        []*VarDecl
	Body        []Stmt
	Line        int
}

// Annotation returns the method's annotation with the given name.
func (m *Method) Annotation(name string) *Annotation {
	for i := range m.Annotations {
		if norm(m.Annotations[i].Name) == norm(name) {
			return &m.Annotations[i]
		}
	}
	return nil
}

type Param struct {
	Name    string
	ByVal   bool
	Default Expr // nil when the parameter is required
}

type Stmt interface{ stmtLine() int }

type (
	AssignStmt struct {
		Target Expr
		Value  Expr
		Line   int
	}
	ExprStmt struct {
		X    Expr
		Line int
	}
	IfStmt struct {
		Conds  []Expr
		Blocks [][]Stmt
		Else   []Stmt
		Line   int
	}
	ForStmt struct {
		Var      string
		From, To Expr
		Body     []Stmt
		Line     int
	}
	ForEachStmt struct {
		Var  string
		In   Expr
		Body []Stmt
		Line int
	}
	WhileStmt struct {
		Cond Expr
		Body []Stmt
		Line int
	}
	TryStmt struct {
		Body   []Stmt
		Except []Stmt
		Line   int
	}
	RaiseStmt struct {
		X    Expr // nil re-raises the current exception
		Line int
	}
	ReturnStmt struct {
		X    Expr
		Line int
	}
	BreakStmt    struct{ Line int }
	ContinueStmt struct{ Line int }
)

func (s *AssignStmt) stmtLine() int   { return s.Line }
func (s *ExprStmt) stmtLine() int     { return s.Line }
func (s *IfStmt) stmtLine() int       { return s.Line }
func (s *ForStmt) stmtLine() int      { return s.Line }
func (s *ForEachStmt) stmtLine() int  { return s.Line }
func (s *WhileStmt) stmtLine() int    { return s.Line }
func (s *TryStmt) stmtLine() int      { return s.Line }
func (s *RaiseStmt) stmtLine() int    { return s.Line }
func (s *ReturnStmt) stmtLine() int   { return s.Line }
func (s *BreakStmt) stmtLine() int    { return s.Line }
func (s *ContinueStmt) stmtLine() int { return s.Line }

type Expr interface{ exprLine() int }

type (
	Literal struct {
		Value Value
		Line  int
	}
	Ident struct {
		Name string
		Line int
	}
	Member struct {
		X    Expr
		Name string
		Line int
	}
	Index struct {
		X, Key Expr
		Line   int
	}
	Call struct {
		Fn   Expr   // *Ident for a global or module call, *Member for a method
		Args []Expr // nil element = omitted argument
		Line int
	}
	NewExpr struct {
		Type     string // empty when the type is computed: Новый(Тип, Параметры)
		TypeExpr Expr
		Args     []Expr
		Line     int
	}
	Unary struct {
		Op   string // "-", "+", "не"
		X    Expr
		Line int
	}
	Binary struct {
		Op   string // + - * / % = <> < > <= >= и или
		L, R Expr
		Line int
	}
	Ternary struct {
		Cond, Then, Else Expr
		Line             int
	}
)

func (e *Literal) exprLine() int { return e.Line }
func (e *Ident) exprLine() int   { return e.Line }
func (e *Member) exprLine() int  { return e.Line }
func (e *Index) exprLine() int   { return e.Line }
func (e *Call) exprLine() int    { return e.Line }
func (e *NewExpr) exprLine() int { return e.Line }
func (e *Unary) exprLine() int   { return e.Line }
func (e *Binary) exprLine() int  { return e.Line }
func (e *Ternary) exprLine() int { return e.Line }
