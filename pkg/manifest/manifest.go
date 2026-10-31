package manifest

import (
	"context"
	"fmt"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/perevod"
)

// Options of one run of a manifest.
type Options struct {
	// Params become the global Структура Параметры.
	Params map[string]string
	// Namespace fills Подразделение of objects that have none.
	Namespace string
	// Message receives what the manifest says through Сообщить().
	Message func(string)
	// MaxSteps bounds the statements a manifest may execute.
	MaxSteps int64
	// Define adds what the caller gives manifests besides the language:
	// the books give Сальдо().
	Define func(*bsl.Interp)
}

// Result is what a manifest posted: objects in the form of the API, in the
// order they were posted.
type Result struct {
	Objects []*bsl.Map
}

// NewInterp makes the global context of manifests: the language, a
// constructor for every kind and Провести().
func NewInterp(res *Result, opts Options) *bsl.Interp {
	it := bsl.New()
	it.MaxSteps = opts.MaxSteps
	if it.MaxSteps == 0 {
		it.MaxSteps = 1_000_000
	}
	if opts.Message != nil {
		it.Message = opts.Message
	}
	DefineKinds(it)
	it.Define(func(_ *bsl.Interp, args []*bsl.Ref) (bsl.Value, error) {
		return perevod.Name(bsl.ToString(bsl.Arg(args, 0))), nil
	}, "ИмяAPI", "APIName")

	params := bsl.NewStructure()
	for k, v := range opts.Params {
		_ = params.Insert(k, v)
	}
	it.SetGlobal(params.Fixed(), "Параметры", "Parameters")
	if opts.Define != nil {
		opts.Define(it)
	}

	it.Define(func(_ *bsl.Interp, args []*bsl.Ref) (bsl.Value, error) {
		for i := range args {
			obj, err := perevod.ToAPI(bsl.Arg(args, i))
			if err != nil {
				return nil, err
			}
			if opts.Namespace != "" {
				fillNamespace(obj, opts.Namespace)
			}
			res.Objects = append(res.Objects, obj)
		}
		return nil, nil
	}, "Провести", "Post")
	return it
}

func fillNamespace(obj *bsl.Map, ns string) {
	k, _ := obj.Lookup("kind")
	kind, ok := perevod.LookupKind(bsl.ToString(k))
	if ok && !kind.Namespaced {
		return
	}
	mdv, _ := obj.Lookup("metadata")
	md, ok := mdv.(*bsl.Map)
	if !ok {
		md = bsl.NewMap()
		obj.Insert("metadata", md)
	}
	if v, ok := md.Lookup("namespace"); !ok || bsl.ToString(v) == "" {
		md.Insert("namespace", perevod.Name(ns))
	}
}

// Run executes a manifest and returns the objects it posted.
func Run(ctx context.Context, name, src string, opts Options) (*Result, error) {
	res := &Result{}
	it := NewInterp(res, opts)
	it.WithContext(ctx)
	if _, err := it.Run(name, src); err != nil {
		return nil, err
	}
	if len(res.Objects) == 0 {
		return nil, fmt.Errorf("манифест ничего не провёл: где Провести(...)?")
	}
	return res, nil
}

// DefineKinds defines a constructor for every kind, «Новый Развёртывание(Имя,
// Подразделение)», and ОбъектКластера for kinds without one.
func DefineKinds(it *bsl.Interp) {
	for i := range perevod.Kinds {
		k := perevod.Kinds[i]
		ctor := func(_ *bsl.Interp, args []bsl.Value) (bsl.Value, error) {
			o := NewObject(k.Ru)
			if k.Native {
				o.Set("ВерсияAPI", perevod.GroupRu+"/v1")
			}
			o.Set("Вид", k.Ru)
			if len(args) > 0 {
				md, _ := o.Get("Метаданные")
				md.(*Object).Set("Имя", bsl.ToString(args[0]))
				if len(args) > 1 && k.Namespaced {
					md.(*Object).Set("Подразделение", bsl.ToString(args[1]))
				}
			}
			return o, nil
		}
		names := append([]string{k.Ru, k.Kind}, k.Aliases...)
		it.DefineType(ctor, names...)
	}
	it.DefineType(func(_ *bsl.Interp, args []bsl.Value) (bsl.Value, error) {
		o := NewObject("ОбъектКластера")
		if len(args) > 0 {
			o.Set("Вид", bsl.ToString(args[0]))
		}
		if len(args) > 1 {
			md, _ := o.Get("Метаданные")
			md.(*Object).Set("Имя", bsl.ToString(args[1]))
		}
		return o, nil
	}, "ОбъектКластера", "ClusterObject")

	it.DefineType(func(_ *bsl.Interp, _ []bsl.Value) (bsl.Value, error) { return &List{}, nil }, "СписокОбъектаКластера")
}
