// Package bsl is an interpreter of the 1C:Enterprise built-in language
// (BSL), the subset that has a meaning outside the 1C platform: the language
// itself, universal collections, strings, dates, decimal numbers and JSON.
package bsl

import "strings"

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokNumber
	tokString
	tokDate
	tokKeyword
	tokOp
	tokAnnotation // &Name
	tokPreproc    // #directive, the rest of the line in text
)

type token struct {
	kind tokenKind
	text string // identifier as written, literal value, operator, directive line
	kw   keyword
	line int
}

type keyword int

const (
	kwNone keyword = iota
	kwIf
	kwThen
	kwElsIf
	kwElse
	kwEndIf
	kwFor
	kwEach
	kwIn
	kwTo
	kwWhile
	kwDo
	kwEndDo
	kwBreak
	kwContinue
	kwProcedure
	kwEndProcedure
	kwFunction
	kwEndFunction
	kwReturn
	kwVar
	kwVal
	kwExport
	kwTry
	kwExcept
	kwEndTry
	kwRaise
	kwNew
	kwAnd
	kwOr
	kwNot
	kwTrue
	kwFalse
	kwUndefined
	kwNull
	kwGoto
	kwExecute
)

var keywords = map[string]keyword{}

func init() {
	for _, k := range []struct {
		kw    keyword
		names []string
	}{
		{kwIf, []string{"если", "if"}},
		{kwThen, []string{"тогда", "then"}},
		{kwElsIf, []string{"иначеесли", "elsif"}},
		{kwElse, []string{"иначе", "else"}},
		{kwEndIf, []string{"конецесли", "endif"}},
		{kwFor, []string{"для", "for"}},
		{kwEach, []string{"каждого", "each"}},
		{kwIn, []string{"из", "in"}},
		{kwTo, []string{"по", "to"}},
		{kwWhile, []string{"пока", "while"}},
		{kwDo, []string{"цикл", "do"}},
		{kwEndDo, []string{"конеццикла", "enddo"}},
		{kwBreak, []string{"прервать", "break"}},
		{kwContinue, []string{"продолжить", "continue"}},
		{kwProcedure, []string{"процедура", "procedure"}},
		{kwEndProcedure, []string{"конецпроцедуры", "endprocedure"}},
		{kwFunction, []string{"функция", "function"}},
		{kwEndFunction, []string{"конецфункции", "endfunction"}},
		{kwReturn, []string{"возврат", "return"}},
		{kwVar, []string{"перем", "var"}},
		{kwVal, []string{"знач", "val"}},
		{kwExport, []string{"экспорт", "export"}},
		{kwTry, []string{"попытка", "try"}},
		{kwExcept, []string{"исключение", "except"}},
		{kwEndTry, []string{"конецпопытки", "endtry"}},
		{kwRaise, []string{"вызватьисключение", "raise"}},
		{kwNew, []string{"новый", "new"}},
		{kwAnd, []string{"и", "and"}},
		{kwOr, []string{"или", "or"}},
		{kwNot, []string{"не", "not"}},
		{kwTrue, []string{"истина", "true"}},
		{kwFalse, []string{"ложь", "false"}},
		{kwUndefined, []string{"неопределено", "undefined"}},
		{kwNull, []string{"null"}},
		{kwGoto, []string{"перейти", "goto"}},
		{kwExecute, []string{"выполнить", "execute"}},
	} {
		for _, n := range k.names {
			keywords[n] = k.kw
		}
	}
}

// norm folds an identifier the way the language compares them: case does
// not matter, and that is all. «Ё» and «Е» stay different letters.
func norm(s string) string { return strings.ToLower(s) }
