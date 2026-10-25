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

// Obrabotka is one operator: its module and what it handles.
type Obrabotka struct {
	Name     string
	Dir      string
	Module   *bsl.Module
	Handlers []Handler // one per kind it reconciles
	Checks   []string  // methods that check objects before the cluster takes them
	Schedule []Scheduled
	CRDs     []string // files of the description of its kinds, Метаданные*.yaml
}

// Handler is what a module does with one kind: the procedure marked
// &Обработка(Вид = …) reconciles it, the one marked &ПередУдалением(Вид = …)
// runs under a finalizer.
type Handler struct {
	Method  string
	Delete  string
	Kind    schema.GroupVersionKind
	Owns    []schema.GroupVersionKind
	Watches []schema.GroupVersionKind
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
		case (ext == ".yaml" || ext == ".yml") && (strings.HasPrefix(n, "Метаданные") || strings.HasPrefix(n, "metadannye")):
			crds = append(crds, filepath.Join(dir, n))
		}
	}
	sort.Strings(modules)
	sort.Strings(crds)
	var file string
	for _, m := range modules {
		base := strings.TrimSuffix(m, filepath.Ext(m))
		// ConfigMap keys cannot be Cyrillic, so the Latin spellings count too
		if base == "Обработка" || base == "Модуль" || base == "obrabotka" || base == "modul" {
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

// declare reads what the module declares in its annotations. Handlers
// may have any names; the names of the platform's events, without
// annotations, still work for a module of one kind.
func (ob *Obrabotka) declare() error {
	if m := ob.Module.Method(HandlerPost); m != nil && m.Annotation("Обработка") == nil {
		return fmt.Errorf("%s: у %s нет аннотации &Обработка(Вид = \"…\")", ob.Name, HandlerPost)
	}
	byKind := map[schema.GroupVersionKind]int{}
	for _, m := range ob.Module.Methods {
		a := m.Annotation("Обработка")
		if a == nil {
			continue
		}
		if v, ok := a.Param("Имя", -1); ok {
			ob.Name = bsl.ToString(v)
		}
		kindV, ok := a.Param("Вид", 0)
		if !ok {
			return fmt.Errorf("%s: в &Обработка у %s не указан Вид", ob.Name, m.Name)
		}
		gvk, err := ResolveGVK(bsl.ToString(kindV))
		if err != nil {
			return fmt.Errorf("%s: %w", ob.Name, err)
		}
		if _, dup := byKind[gvk]; dup {
			return fmt.Errorf("%s: вид «%s» обрабатывают две процедуры", ob.Name, bsl.ToString(kindV))
		}
		h := Handler{Method: m.Name, Kind: gvk}
		for _, p := range []struct {
			name string
			into *[]schema.GroupVersionKind
		}{{"Владеет", &h.Owns}, {"Наблюдает", &h.Watches}} {
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
		byKind[gvk] = len(ob.Handlers)
		ob.Handlers = append(ob.Handlers, h)
	}
	for _, m := range ob.Module.Methods {
		a := m.Annotation("ПередУдалением")
		switch {
		case a != nil:
			kindV, ok := a.Param("Вид", 0)
			if !ok {
				return fmt.Errorf("%s: в &ПередУдалением у %s не указан Вид", ob.Name, m.Name)
			}
			gvk, err := ResolveGVK(bsl.ToString(kindV))
			if err != nil {
				return err
			}
			i, ok := byKind[gvk]
			if !ok {
				return fmt.Errorf("%s: &ПередУдалением для вида «%s», который обработка не ведёт", ob.Name, bsl.ToString(kindV))
			}
			ob.Handlers[i].Delete = m.Name
		case norm(m.Name) == norm(HandlerDelete):
			for i := range ob.Handlers {
				if norm(ob.Handlers[i].Method) == norm(HandlerPost) {
					ob.Handlers[i].Delete = m.Name
				}
			}
		}
		if m.Annotation("ПроверкаЗаполнения") != nil || norm(m.Name) == norm(HandlerValidate) {
			ob.Checks = append(ob.Checks, m.Name)
		}
		a = m.Annotation("Расписание")
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
	if len(ob.Handlers) == 0 && len(ob.Schedule) == 0 && len(ob.Checks) == 0 {
		return fmt.Errorf("%s: обработка ничего не обрабатывает: нет ни &Обработка, ни проверок заполнения, ни &Расписание", ob.Name)
	}
	return nil
}

func norm(s string) string { return strings.ToLower(s) }

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
