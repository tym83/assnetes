package bsl

import (
	"fmt"
	"strings"
	"unicode"
)

type lexer struct {
	src  []rune
	pos  int
	line int
	toks []token
}

func lex(src string) ([]token, error) {
	l := &lexer{src: []rune(strings.TrimPrefix(src, "\uFEFF")), line: 1}
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		l.toks = append(l.toks, t)
		if t.kind == tokEOF {
			return l.toks, nil
		}
	}
}

func (l *lexer) peek(off int) rune {
	if l.pos+off < len(l.src) {
		return l.src[l.pos+off]
	}
	return 0
}

func (l *lexer) errorf(format string, args ...any) error {
	return &SyntaxError{Line: l.line, Message: fmt.Sprintf(format, args...)}
}

func isIdentStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }
func isIdentPart(r rune) bool  { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// atLineStart reports whether only blanks precede pos on its line.
func (l *lexer) atLineStart() bool {
	for i := l.pos - 1; i >= 0; i-- {
		switch l.src[i] {
		case '\n':
			return true
		case ' ', '\t', '\r':
		default:
			return false
		}
	}
	return true
}

func (l *lexer) prevIsDot() bool {
	for i := len(l.toks) - 1; i >= 0; i-- {
		return l.toks[i].kind == tokOp && l.toks[i].text == "."
	}
	return false
}

func (l *lexer) next() (token, error) {
	for l.pos < len(l.src) {
		r := l.src[l.pos]
		switch {
		case r == '\n':
			l.line++
			l.pos++
		case r == ' ' || r == '\t' || r == '\r' || r == ' ':
			l.pos++
		case r == '/' && l.peek(1) == '/':
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
		default:
			return l.scan()
		}
	}
	return token{kind: tokEOF, line: l.line}, nil
}

func (l *lexer) scan() (token, error) {
	r := l.src[l.pos]
	line := l.line
	switch {
	case r == '#' && l.atLineStart():
		start := l.pos + 1
		for l.pos < len(l.src) && l.src[l.pos] != '\n' {
			l.pos++
		}
		text := string(l.src[start:l.pos])
		if i := strings.Index(text, "//"); i >= 0 {
			text = text[:i]
		}
		return token{kind: tokPreproc, text: strings.TrimSpace(text), line: line}, nil
	case r == '&':
		l.pos++
		start := l.pos
		for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
			l.pos++
		}
		if start == l.pos {
			return token{}, l.errorf("ожидается имя аннотации после «&»")
		}
		return token{kind: tokAnnotation, text: string(l.src[start:l.pos]), line: line}, nil
	case isIdentStart(r):
		start := l.pos
		for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
			l.pos++
		}
		text := string(l.src[start:l.pos])
		if kw, ok := keywords[norm(text)]; ok && !l.prevIsDot() {
			return token{kind: tokKeyword, text: text, kw: kw, line: line}, nil
		}
		return token{kind: tokIdent, text: text, line: line}, nil
	case unicode.IsDigit(r):
		start := l.pos
		for l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
			l.pos++
		}
		if l.peek(0) == '.' && unicode.IsDigit(l.peek(1)) {
			l.pos++
			for l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
				l.pos++
			}
		}
		return token{kind: tokNumber, text: string(l.src[start:l.pos]), line: line}, nil
	case r == '"':
		s, err := l.scanString()
		return token{kind: tokString, text: s, line: line}, err
	case r == '\'':
		l.pos++
		var digits []rune
		for l.pos < len(l.src) && l.src[l.pos] != '\'' {
			if l.src[l.pos] == '\n' {
				return token{}, l.errorf("незавершённый литерал даты")
			}
			if unicode.IsDigit(l.src[l.pos]) {
				digits = append(digits, l.src[l.pos])
			}
			l.pos++
		}
		if l.pos >= len(l.src) {
			return token{}, l.errorf("незавершённый литерал даты")
		}
		l.pos++
		return token{kind: tokDate, text: string(digits), line: line}, nil
	}
	for _, op := range []string{"<>", "<=", ">="} {
		if string(r)+string(l.peek(1)) == op {
			l.pos += 2
			return token{kind: tokOp, text: op, line: line}, nil
		}
	}
	if strings.ContainsRune("+-*/%=<>()[],;.?:~", r) {
		l.pos++
		return token{kind: tokOp, text: string(r), line: line}, nil
	}
	return token{}, l.errorf("недопустимый символ «%c»", r)
}

// scanString reads a string literal. A literal continues on the next line
// when that line starts with «|»; doubled quotes stand for one quote.
func (l *lexer) scanString() (string, error) {
	var b strings.Builder
	l.pos++ // opening quote
	for {
		if l.pos >= len(l.src) {
			return "", l.errorf("незавершённая строка")
		}
		r := l.src[l.pos]
		switch r {
		case '"':
			if l.peek(1) == '"' {
				b.WriteRune('"')
				l.pos += 2
				continue
			}
			l.pos++
			return b.String(), nil
		case '\n':
			b.WriteRune('\n')
			l.line++
			l.pos++
			if err := l.skipToContinuation(); err != nil {
				return "", err
			}
		case '\r':
			l.pos++
		default:
			b.WriteRune(r)
			l.pos++
		}
	}
}

func (l *lexer) skipToContinuation() error {
	for l.pos < len(l.src) {
		switch r := l.src[l.pos]; {
		case r == ' ' || r == '\t' || r == '\r':
			l.pos++
		case r == '/' && l.peek(1) == '/':
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
		case r == '\n':
			l.line++
			l.pos++
		case r == '|':
			l.pos++
			return nil
		default:
			return l.errorf("строка продолжается на новой строке без «|»")
		}
	}
	return l.errorf("незавершённая строка")
}

// SyntaxError is a compile-time error, worded the way the platform words it.
type SyntaxError struct {
	Module  string
	Line    int
	Message string
}

func (e *SyntaxError) Error() string {
	mod := e.Module
	if mod == "" {
		mod = "Модуль"
	}
	return fmt.Sprintf("{%s(%d)}: Синтаксическая ошибка: %s", mod, e.Line, e.Message)
}
