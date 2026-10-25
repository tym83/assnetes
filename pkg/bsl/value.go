package bsl

import (
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Value is a value of the language: Undefined, Null, bool, Number, string,
// time.Time (a Дата, always without a time zone), TypeValue or Object.
type Value any

type Undefined struct{}
type Null struct{}

// TypeValue is what Тип() and ТипЗнч() return.
type TypeValue struct{ Name string }

// Object is a value of a reference type: a collection or a host object.
type Object interface {
	TypeName() string
}

// Getter and Setter give an object properties: Объект.Свойство.
type Getter interface {
	Get(name string) (Value, bool)
}

type Setter interface {
	Set(name string, v Value) error
}

// Caller gives an object methods. A method gets its arguments as references,
// so it can write output parameters, as Структура.Свойство() does.
type Caller interface {
	CallMethod(it *Interp, name string, args []*Ref) (Value, error)
}

// Indexer gives an object Объект[Ключ].
type Indexer interface {
	GetIndex(key Value) (Value, error)
	SetIndex(key, v Value) error
}

// Iterable lets Для Каждого walk an object.
type Iterable interface {
	Items() []Value
}

// Ref is a cell holding a value, the way arguments are passed by reference.
type Ref struct{ V Value }

// Arg returns the value of argument i, Undefined when it was omitted.
func Arg(args []*Ref, i int) Value {
	if i < len(args) && args[i] != nil && args[i].V != nil {
		return args[i].V
	}
	return Undefined{}
}

// Has reports whether argument i was given.
func Has(args []*Ref, i int) bool {
	if i >= len(args) || args[i] == nil || args[i].V == nil {
		return false
	}
	_, undef := args[i].V.(Undefined)
	return !undef
}

// Number is the decimal number of the language: 0.1 + 0.2 = 0.3.
type Number struct{ r *big.Rat }

func NewInt(i int64) Number { return Number{new(big.Rat).SetInt64(i)} }

// NewFloat makes a Number from a float by its shortest decimal form.
func NewFloat(f float64) Number {
	n, err := ParseNumber(strings.TrimSuffix(fmt.Sprintf("%v", f), ".0"))
	if err != nil {
		r := new(big.Rat)
		r.SetFloat64(f)
		return Number{r}
	}
	return n
}

func ParseNumber(s string) (Number, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return Number{}, fmt.Errorf("неверное число «%s»", s)
	}
	return Number{r}, nil
}

func (n Number) rat() *big.Rat {
	if n.r == nil {
		return new(big.Rat)
	}
	return n.r
}

func (n Number) Add(m Number) Number { return Number{new(big.Rat).Add(n.rat(), m.rat())} }
func (n Number) Sub(m Number) Number { return Number{new(big.Rat).Sub(n.rat(), m.rat())} }
func (n Number) Mul(m Number) Number { return Number{new(big.Rat).Mul(n.rat(), m.rat())} }
func (n Number) Neg() Number         { return Number{new(big.Rat).Neg(n.rat())} }
func (n Number) Cmp(m Number) int    { return n.rat().Cmp(m.rat()) }
func (n Number) Sign() int           { return n.rat().Sign() }
func (n Number) IsInt() bool         { return n.rat().IsInt() }

func (n Number) Div(m Number) (Number, error) {
	if m.Sign() == 0 {
		return Number{}, fmt.Errorf("Деление на 0")
	}
	return Number{new(big.Rat).Quo(n.rat(), m.rat())}, nil
}

// Mod is the remainder with the sign of the dividend, as in the platform.
func (n Number) Mod(m Number) (Number, error) {
	if m.Sign() == 0 {
		return Number{}, fmt.Errorf("Деление на 0")
	}
	q, _ := n.Div(m)
	return n.Sub(q.Trunc().Mul(m)), nil
}

// Trunc drops the fraction: Цел().
func (n Number) Trunc() Number {
	r := n.rat()
	q := new(big.Int).Quo(r.Num(), r.Denom())
	return Number{new(big.Rat).SetInt(q)}
}

// Round rounds to digits after the point, halves away from zero: Окр().
func (n Number) Round(digits int) Number {
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(digits))), nil))
	x := new(big.Rat).Set(n.rat())
	if digits >= 0 {
		x.Mul(x, scale)
	} else {
		x.Quo(x, scale)
	}
	half := big.NewRat(1, 2)
	if x.Sign() < 0 {
		x.Sub(x, half)
	} else {
		x.Add(x, half)
	}
	t := Number{x}.Trunc().rat()
	if digits >= 0 {
		t.Quo(t, scale)
	} else {
		t.Mul(t, scale)
	}
	return Number{t}
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

func (n Number) Int64() int64 {
	t := n.Trunc().rat()
	return t.Num().Int64()
}

func (n Number) Float64() float64 {
	f, _ := n.rat().Float64()
	return f
}

// Plain is the machine form: digits and a point, at most 27 decimals.
func (n Number) Plain() string {
	if n.IsInt() {
		return n.rat().Num().String()
	}
	s := n.rat().FloatString(27)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

func (n Number) String() string { return FormatNumber(n, "") }

// TypeOf names the type of a value the way ТипЗнч() does.
func TypeOf(v Value) string {
	switch x := v.(type) {
	case nil, Undefined:
		return "Неопределено"
	case Null:
		return "Null"
	case bool:
		return "Булево"
	case Number:
		return "Число"
	case string:
		return "Строка"
	case time.Time:
		return "Дата"
	case TypeValue:
		return "Тип"
	case Object:
		return x.TypeName()
	}
	return fmt.Sprintf("%T", v)
}

// ToString converts a value to a string the way Строка() does, numbers in
// the Russian locale: Строка(1000.5) = "1 000,5".
func ToString(v Value) string {
	switch x := v.(type) {
	case nil, Undefined, Null:
		return ""
	case bool:
		if x {
			return "Да"
		}
		return "Нет"
	case Number:
		return FormatNumber(x, "")
	case string:
		return x
	case time.Time:
		if x.IsZero() || x.Equal(emptyDate) {
			return ""
		}
		return x.Format("02.01.2006 15:04:05")
	case TypeValue:
		return x.Name
	case interface{ String() string }:
		return x.String()
	case Object:
		return x.TypeName()
	}
	return fmt.Sprint(v)
}

var emptyDate = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)

// EmptyDate is '00010101', the date nobody filled in.
func EmptyDate() time.Time { return emptyDate }

func parseDateDigits(d string) (time.Time, error) {
	if len(d) != 8 && len(d) != 12 && len(d) != 14 {
		return time.Time{}, fmt.Errorf("неверный литерал даты '%s'", d)
	}
	d += strings.Repeat("0", 14-len(d))
	t, err := time.Parse("20060102150405", d)
	if err != nil {
		if strings.Trim(d, "0") == "1" || d == "00010101000000" || d == "00000000000000" {
			return emptyDate, nil
		}
		return time.Time{}, fmt.Errorf("неверный литерал даты '%s'", d)
	}
	return t, nil
}

// Equal is the «=» of the language: values of different types are not
// equal, strings compare by case, objects by identity.
func Equal(a, b Value) bool {
	switch x := a.(type) {
	case nil, Undefined:
		switch b.(type) {
		case nil, Undefined:
			return true
		}
		return false
	case Null:
		_, ok := b.(Null)
		return ok
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case Number:
		y, ok := b.(Number)
		return ok && x.Cmp(y) == 0
	case string:
		y, ok := b.(string)
		return ok && x == y
	case time.Time:
		y, ok := b.(time.Time)
		return ok && x.Equal(y)
	case TypeValue:
		y, ok := b.(TypeValue)
		return ok && norm(x.Name) == norm(y.Name)
	}
	if e, ok := a.(interface{ EqualTo(Value) bool }); ok {
		return e.EqualTo(b)
	}
	return a == b
}

// Filled is ЗначениеЗаполнено().
func Filled(v Value) bool {
	switch x := v.(type) {
	case nil, Undefined, Null:
		return false
	case bool:
		return true
	case Number:
		return x.Sign() != 0
	case string:
		return strings.TrimSpace(x) != ""
	case time.Time:
		return !x.Equal(emptyDate) && !x.IsZero()
	case interface{ Count() int }:
		return x.Count() > 0
	}
	return true
}
