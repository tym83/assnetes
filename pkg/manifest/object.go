// Package manifest runs manifests written in the 1C language: modules that
// build objects of the cluster and «проводят» them.
package manifest

import (
	"fmt"
	"strings"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/perevod"
)

// listFields are fields that hold lists: reading one that is not there
// gives an empty list rather than an object, so Контейнеры.Добавить() works.
var listFields = map[string]bool{
	"containers": true, "initContainers": true, "ports": true, "env": true, "envFrom": true,
	"volumes": true, "volumeMounts": true, "args": true, "command": true, "tolerations": true,
	"rules": true, "paths": true, "tls": true, "hosts": true, "accessModes": true, "subjects": true,
	"verbs": true, "apiGroups": true, "finalizers": true, "ownerReferences": true,
	"imagePullSecrets": true, "items": true, "matchExpressions": true, "values": true,
}

func fold(s string) string { return strings.ToLower(s) }

// Object is ОбъектКластера: an ordered set of fields that grows as it is
// written, so «Развёртывание.Спецификация.Шаблон.Спецификация» needs no
// declarations on the way. Fields read but never written stay empty and
// are left out of the object the cluster gets.
type Object struct {
	typeName string
	order    []string
	names    map[string]string
	vals     map[string]bsl.Value
}

func NewObject(typeName string) *Object {
	return &Object{typeName: typeName, names: map[string]string{}, vals: map[string]bsl.Value{}}
}

func (o *Object) TypeName() string { return o.typeName }

func (o *Object) Count() int { return len(o.order) }

func (o *Object) put(name string, v bsl.Value) {
	k := fold(name)
	if _, ok := o.vals[k]; !ok {
		o.order = append(o.order, k)
		o.names[k] = name
	}
	o.vals[k] = v
}

func (o *Object) Get(name string) (bsl.Value, bool) {
	if v, ok := o.vals[fold(name)]; ok {
		return v, true
	}
	var child bsl.Value
	if listFields[perevod.FieldToAPI(name)] {
		child = &List{}
	} else {
		child = NewObject("ОбъектКластера")
	}
	o.put(name, child)
	return child, true
}

func (o *Object) Set(name string, v bsl.Value) error {
	o.put(name, v)
	return nil
}

func (o *Object) GetIndex(k bsl.Value) (bsl.Value, error) {
	v, _ := o.Get(bsl.ToString(k))
	return v, nil
}

func (o *Object) SetIndex(k, v bsl.Value) error { return o.Set(bsl.ToString(k), v) }

// Entries lists the fields that hold something.
func (o *Object) Entries() ([]string, []bsl.Value) {
	var keys []string
	var vals []bsl.Value
	for _, k := range o.order {
		v := o.vals[k]
		if empty(v) {
			continue
		}
		keys = append(keys, o.names[k])
		vals = append(vals, v)
	}
	return keys, vals
}

func empty(v bsl.Value) bool {
	switch x := v.(type) {
	case *Object:
		keys, _ := x.Entries()
		return len(keys) == 0
	case *List:
		return len(x.elems) == 0
	case bsl.Undefined, nil:
		return true
	}
	return false
}

func (o *Object) Items() []bsl.Value {
	keys, vals := o.Entries()
	out := make([]bsl.Value, len(keys))
	for i := range keys {
		out[i] = &bsl.KeyValue{Key: keys[i], Value: vals[i]}
	}
	return out
}

func (o *Object) CallMethod(_ *bsl.Interp, name string, args []*bsl.Ref) (bsl.Value, error) {
	switch fold(name) {
	case "вставить", "insert":
		o.put(bsl.ToString(bsl.Arg(args, 0)), bsl.Arg(args, 1))
		return nil, nil
	case "получить", "get":
		if v, ok := o.vals[fold(bsl.ToString(bsl.Arg(args, 0)))]; ok {
			return v, nil
		}
		return bsl.Undefined{}, nil
	case "свойство", "property":
		v, ok := o.vals[fold(bsl.ToString(bsl.Arg(args, 0)))]
		ok = ok && !empty(v)
		if len(args) > 1 && args[1] != nil {
			if !ok {
				v = bsl.Undefined{}
			}
			args[1].V = v
		}
		return ok, nil
	case "удалить", "delete":
		k := fold(bsl.ToString(bsl.Arg(args, 0)))
		delete(o.vals, k)
		for i, x := range o.order {
			if x == k {
				o.order = append(o.order[:i], o.order[i+1:]...)
				break
			}
		}
		return nil, nil
	case "количество", "count":
		keys, _ := o.Entries()
		return bsl.NewInt(int64(len(keys))), nil
	}
	return nil, fmt.Errorf("Метод объекта не обнаружен (%s)", name)
}

// List is a list field of an object.
type List struct{ elems []bsl.Value }

func (*List) TypeName() string { return "СписокОбъектаКластера" }

func (l *List) Items() []bsl.Value { return append([]bsl.Value(nil), l.elems...) }

func (l *List) Count() int { return len(l.elems) }

func (l *List) GetIndex(k bsl.Value) (bsl.Value, error) {
	n, ok := k.(bsl.Number)
	i := int(n.Int64())
	if !ok || i < 0 || i >= len(l.elems) {
		return nil, fmt.Errorf("Индекс находится за границами диапазона")
	}
	return l.elems[i], nil
}

func (l *List) SetIndex(k, v bsl.Value) error {
	n, ok := k.(bsl.Number)
	i := int(n.Int64())
	if !ok || i < 0 || i >= len(l.elems) {
		return fmt.Errorf("Индекс находится за границами диапазона")
	}
	l.elems[i] = v
	return nil
}

func (l *List) CallMethod(_ *bsl.Interp, name string, args []*bsl.Ref) (bsl.Value, error) {
	switch fold(name) {
	case "добавить", "add":
		if bsl.Has(args, 0) {
			l.elems = append(l.elems, bsl.Arg(args, 0))
			return nil, nil
		}
		o := NewObject("ОбъектКластера")
		l.elems = append(l.elems, o)
		return o, nil
	case "количество", "count":
		return bsl.NewInt(int64(len(l.elems))), nil
	case "очистить", "clear":
		l.elems = nil
		return nil, nil
	}
	return nil, fmt.Errorf("Метод объекта не обнаружен (%s)", name)
}

// FromValue turns a value of Соответствия and Массивы into objects that grow
// as they are written, the way handlers of operators get them.
func FromValue(v bsl.Value) bsl.Value {
	switch x := v.(type) {
	case *bsl.Map:
		o := NewObject("ОбъектКластера")
		for _, k := range x.Keys() {
			val, _ := x.Lookup(k)
			o.put(bsl.ToString(k), FromValue(val))
		}
		return o
	case *bsl.Structure:
		o := NewObject("ОбъектКластера")
		for _, k := range x.Keys() {
			val, _ := x.Get(k)
			o.put(k, FromValue(val))
		}
		return o
	case *bsl.Array:
		l := &List{}
		for _, e := range x.Elems {
			l.elems = append(l.elems, FromValue(e))
		}
		return l
	}
	return v
}

// WithType names the type of an object, so ТипЗнч() tells kinds apart.
func (o *Object) WithType(name string) *Object {
	o.typeName = name
	return o
}
