// Package obrabotka runs Kubernetes operators written in the 1C language.
// An operator is an «обработка»: a module whose event handlers are named
// after the events of an object module of the platform:
//
//	ОбработкаПроведения(Объект, Отказ)     — bring the world to the object
//	ПередУдалением(Объект, Отказ)          — clean up before the object goes
//	ОбработкаПроверкиЗаполнения(Объект, Отказ) — reject an invalid object
//	ПередЗаписью(Объект)                   — fill in what was left out
//	&Расписание("…") Процедура …()         — scheduled work
//
// The runtime does the rest: watches, queues, retries, server-side apply,
// finalizers, events, status and leader election.
package obrabotka

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/perevod"
)

// Handler names, as the platform names the events of an object module.
const (
	HandlerPost     = "ОбработкаПроведения"
	HandlerDelete   = "ПередУдалением"
	HandlerValidate = "ОбработкаПроверкиЗаполнения"
	HandlerDefault  = "ПередЗаписью"
)

// Obrabotka is one operator: its module and what it watches.
type Obrabotka struct {
	Name     string
	Dir      string
	Module   *bsl.Module
	Kind     schema.GroupVersionKind // the kind it reconciles; empty for schedule-only
	Owns     []schema.GroupVersionKind
	Watches  []schema.GroupVersionKind
	Schedule []Scheduled
	CRDs     []string // files of the description of its kinds, Метаданные*.yaml
}

// Scheduled is a procedure run on a schedule.
type Scheduled struct {
	Method string
	Cron   string
}

// Load reads an обработка from a directory: one module, Обработка.bsl or
// Модуль.bsl or the only .bsl/.os file, and the descriptions of its kinds.
func Load(dir string) (*Obrabotka, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var modules, crds []string
	for _, e := range entries {
		n := e.Name()
		switch ext := strings.ToLower(filepath.Ext(n)); {
		case ext == ".bsl" || ext == ".os":
			modules = append(modules, n)
		case (ext == ".yaml" || ext == ".yml") && strings.HasPrefix(n, "Метаданные"):
			crds = append(crds, filepath.Join(dir, n))
		}
	}
	sort.Strings(modules)
	sort.Strings(crds)
	var file string
	for _, m := range modules {
		base := strings.TrimSuffix(m, filepath.Ext(m))
		if base == "Обработка" || base == "Модуль" || base == "ObjectModule" {
			file = m
		}
	}
	if file == "" && len(modules) == 1 {
		file = modules[0]
	}
	if file == "" {
		return nil, fmt.Errorf("%s: не найден модуль обработки (Обработка.bsl)", dir)
	}
	src, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return nil, err
	}
	name := filepath.Base(dir)
	mod, err := bsl.Compile(name, string(src))
	if err != nil {
		return nil, err
	}
	ob := &Obrabotka{Name: name, Dir: dir, Module: mod, CRDs: crds}
	return ob, ob.declare()
}

// declare reads what the module declares in its annotations.
func (ob *Obrabotka) declare() error {
	if m := ob.Module.Method(HandlerPost); m != nil {
		a := m.Annotation("Обработка")
		if a == nil {
			return fmt.Errorf("%s: у %s нет аннотации &Обработка(Вид = \"…\")", ob.Name, HandlerPost)
		}
		if v, ok := a.Param("Имя", -1); ok {
			ob.Name = bsl.ToString(v)
		}
		kindV, ok := a.Param("Вид", 0)
		if !ok {
			return fmt.Errorf("%s: в &Обработка не указан Вид", ob.Name)
		}
		gvk, err := ResolveGVK(bsl.ToString(kindV))
		if err != nil {
			return fmt.Errorf("%s: %w", ob.Name, err)
		}
		ob.Kind = gvk
		for _, p := range []struct {
			name string
			into *[]schema.GroupVersionKind
		}{{"Владеет", &ob.Owns}, {"Наблюдает", &ob.Watches}} {
			v, ok := a.Param(p.name, -1)
			if !ok {
				continue
			}
			for _, k := range strings.Split(bsl.ToString(v), ",") {
				if k = strings.TrimSpace(k); k == "" {
					continue
				}
				g, err := ResolveGVK(k)
				if err != nil {
					return fmt.Errorf("%s: %s: %w", ob.Name, p.name, err)
				}
				*p.into = append(*p.into, g)
			}
		}
	}
	for _, m := range ob.Module.Methods {
		a := m.Annotation("Расписание")
		if a == nil {
			continue
		}
		v, ok := a.Param("Cron", 0)
		if !ok {
			return fmt.Errorf("%s: у %s в &Расписание не указано расписание", ob.Name, m.Name)
		}
		if len(m.Params) > 0 {
			return fmt.Errorf("%s: регламентная процедура %s не принимает параметров", ob.Name, m.Name)
		}
		ob.Schedule = append(ob.Schedule, Scheduled{Method: m.Name, Cron: bsl.ToString(v)})
	}
	if ob.Kind.Empty() && len(ob.Schedule) == 0 {
		return fmt.Errorf("%s: обработка ничего не обрабатывает: нет ни %s, ни &Расписание", ob.Name, HandlerPost)
	}
	return nil
}

// ResolveGVK finds a kind by its Russian or English name, or takes
// «группа/версия/Вид» as it is.
func ResolveGVK(name string) (schema.GroupVersionKind, error) {
	if k, ok := perevod.LookupKind(name); ok {
		gv, err := schema.ParseGroupVersion(k.APIVersion)
		if err != nil {
			return schema.GroupVersionKind{}, err
		}
		return gv.WithKind(k.Kind), nil
	}
	parts := strings.Split(name, "/")
	switch len(parts) {
	case 2:
		return schema.GroupVersionKind{Version: parts[0], Kind: parts[1]}, nil
	case 3:
		group := strings.TrimSuffix(perevod.GroupToAPI(parts[0]+"/v"), "/v")
		return schema.GroupVersionKind{Group: group, Version: parts[1], Kind: parts[2]}, nil
	}
	return schema.GroupVersionKind{}, fmt.Errorf("вид «%s» нам не знаком", name)
}
