// Package metadannye turns a description of a kind, written the way 1C
// describes metadata — реквизиты with types — into a CustomResourceDefinition
// whose fields are Cyrillic in the API itself.
//
//	Вид: ИнформационнаяБаза
//	ВидAPI: Infobase
//	Множественное: infobases
//	ВПодразделении: Да
//	Реквизиты:
//	  Конфигурация: {Тип: Строка, Обязательный: Да}
//	  Пользователей: {Тип: Число, Минимум: 1, ПоУмолчанию: 10}
//	Состояние:
//	  Фаза: Строка
package metadannye

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tym83/assnetes/pkg/perevod"
)

// Description is the description of one kind.
type Description struct {
	Ru         string
	Kind       string
	Plural     string
	Namespaced bool
	ShortNames []string
	Doc        string
	Spec       []Field
	Status     []Field
	Columns    []Column
}

// Field is a реквизит: a name and its type.
type Field struct {
	Name     string
	Type     string // Строка, Число, Целое, Булево, Дата, Массив, Структура, Соответствие, Произвольный
	Required bool
	Doc      string
	Min, Max *float64
	Default  any
	Enum     []string
	Pattern  string
	Elem     *Field  // of a Массив
	Fields   []Field // of a Структура
}

// Column is a column of «1сctl получить» and «kubectl get».
type Column struct {
	Name string
	Path string
	Type string
}

// Parse reads a description from YAML.
func Parse(data []byte) (*Description, error) {
	var n yaml.Node
	if err := yaml.Unmarshal(data, &n); err != nil {
		return nil, fmt.Errorf("описание вида: %w", err)
	}
	if n.Kind != yaml.DocumentNode || len(n.Content) == 0 || n.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("описание вида должно быть словарём")
	}
	d := &Description{}
	top := n.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		k, v := top.Content[i].Value, top.Content[i+1]
		var err error
		switch k {
		case "Вид":
			d.Ru = v.Value
		case "ВидAPI":
			d.Kind = v.Value
		case "Множественное":
			d.Plural = v.Value
		case "ВПодразделении":
			d.Namespaced = yes(v.Value)
		case "Описание":
			d.Doc = v.Value
		case "Сокращения":
			for _, c := range v.Content {
				d.ShortNames = append(d.ShortNames, c.Value)
			}
		case "Реквизиты":
			d.Spec, err = fields(v)
		case "Состояние":
			d.Status, err = fields(v)
		case "Колонки":
			for _, c := range v.Content {
				col := Column{Type: "string"}
				for j := 0; j+1 < len(c.Content); j += 2 {
					switch c.Content[j].Value {
					case "Имя":
						col.Name = c.Content[j+1].Value
					case "Путь":
						col.Path = c.Content[j+1].Value
					case "Тип":
						col.Type = jsonType(c.Content[j+1].Value)
					}
				}
				d.Columns = append(d.Columns, col)
			}
		default:
			return nil, fmt.Errorf("описание вида: неизвестный раздел «%s»", k)
		}
		if err != nil {
			return nil, err
		}
	}
	if d.Kind == "" || d.Plural == "" {
		return nil, fmt.Errorf("описание вида «%s»: нужны ВидAPI и Множественное", d.Ru)
	}
	return d, nil
}

func yes(s string) bool {
	switch strings.ToLower(s) {
	case "да", "истина", "true", "yes":
		return true
	}
	return false
}

func fields(n *yaml.Node) ([]Field, error) {
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("строка %d: ожидается список реквизитов", n.Line)
	}
	var out []Field
	for i := 0; i+1 < len(n.Content); i += 2 {
		f, err := field(n.Content[i].Value, n.Content[i+1])
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func field(name string, n *yaml.Node) (Field, error) {
	f := Field{Name: name}
	if n.Kind == yaml.ScalarNode {
		f.Type = n.Value
		return f, check(&f, n.Line)
	}
	if n.Kind != yaml.MappingNode {
		return f, fmt.Errorf("строка %d: реквизит «%s»: ожидается тип или словарь", n.Line, name)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		switch k {
		case "Тип":
			f.Type = v.Value
		case "Обязательный":
			f.Required = yes(v.Value)
		case "Описание":
			f.Doc = v.Value
		case "Минимум", "Максимум":
			var x float64
			if err := v.Decode(&x); err != nil {
				return f, fmt.Errorf("строка %d: %s: %w", v.Line, k, err)
			}
			if k == "Минимум" {
				f.Min = &x
			} else {
				f.Max = &x
			}
		case "ПоУмолчанию":
			var x any
			if err := v.Decode(&x); err != nil {
				return f, err
			}
			f.Default = x
		case "Значения":
			for _, c := range v.Content {
				f.Enum = append(f.Enum, c.Value)
			}
		case "Шаблон":
			f.Pattern = v.Value
		case "Элемент":
			e, err := field(name+"[]", v)
			if err != nil {
				return f, err
			}
			f.Elem = &e
		case "Реквизиты":
			fs, err := fields(v)
			if err != nil {
				return f, err
			}
			f.Fields = fs
		default:
			return f, fmt.Errorf("строка %d: реквизит «%s»: неизвестное свойство «%s»", v.Line, name, k)
		}
	}
	return f, check(&f, n.Line)
}

func check(f *Field, line int) error {
	switch f.Type {
	case "Строка", "Число", "Целое", "Булево", "Дата", "Соответствие", "Произвольный":
	case "Массив":
		if f.Elem == nil {
			f.Elem = &Field{Type: "Строка"}
		}
	case "Структура":
	case "":
		if len(f.Fields) > 0 {
			f.Type = "Структура"
			return nil
		}
		return fmt.Errorf("строка %d: у реквизита «%s» не указан Тип", line, f.Name)
	default:
		return fmt.Errorf("строка %d: реквизит «%s»: неизвестный тип «%s»", line, f.Name, f.Type)
	}
	return nil
}

func jsonType(t string) string {
	switch t {
	case "Число":
		return "number"
	case "Целое":
		return "integer"
	case "Булево":
		return "boolean"
	case "Дата":
		return "date"
	}
	return "string"
}

func schema(f Field) map[string]any {
	s := map[string]any{}
	if f.Doc != "" {
		s["description"] = f.Doc
	}
	switch f.Type {
	case "Строка":
		s["type"] = "string"
	case "Число":
		s["type"] = "number"
	case "Целое":
		s["type"] = "integer"
	case "Булево":
		s["type"] = "boolean"
	case "Дата":
		s["type"] = "string"
		s["format"] = "date-time"
	case "Массив":
		s["type"] = "array"
		s["items"] = schema(*f.Elem)
	case "Соответствие":
		s["type"] = "object"
		s["additionalProperties"] = map[string]any{"type": "string"}
	case "Произвольный":
		s["x-kubernetes-preserve-unknown-fields"] = true
	case "Структура":
		s["type"] = "object"
		props, req := properties(f.Fields)
		s["properties"] = props
		if len(req) > 0 {
			s["required"] = req
		}
	}
	if f.Min != nil {
		s["minimum"] = *f.Min
	}
	if f.Max != nil {
		s["maximum"] = *f.Max
	}
	if f.Default != nil {
		s["default"] = defaultOf(f)
	}
	if len(f.Enum) > 0 {
		s["enum"] = f.Enum
	}
	if f.Pattern != "" {
		s["pattern"] = f.Pattern
	}
	return s
}

func properties(fs []Field) (map[string]any, []string) {
	props := map[string]any{}
	var req []string
	for _, f := range fs {
		props[f.Name] = schema(f)
		if f.Required {
			req = append(req, f.Name)
		}
	}
	sort.Strings(req)
	return props, req
}

// CRD returns the CustomResourceDefinition of the kind as JSON.
func (d *Description) CRD() ([]byte, error) {
	spec, specReq := properties(d.Spec)
	status, _ := properties(d.Status)
	root := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"apiVersion": map[string]any{"type": "string"},
			"kind":       map[string]any{"type": "string"},
			"metadata":   map[string]any{"type": "object"},
			"spec":       withRequired(map[string]any{"type": "object", "properties": spec}, specReq),
			"status":     map[string]any{"type": "object", "properties": status, "x-kubernetes-preserve-unknown-fields": true},
		},
	}
	if d.Doc != "" {
		root["description"] = d.Doc
	}
	scope := "Cluster"
	if d.Namespaced {
		scope = "Namespaced"
	}
	cols := []map[string]any{}
	for _, c := range d.Columns {
		cols = append(cols, map[string]any{"name": c.Name, "type": c.Type, "jsonPath": c.Path})
	}
	cols = append(cols, map[string]any{"name": "Возраст", "type": "date", "jsonPath": ".metadata.creationTimestamp"})
	names := map[string]any{
		"kind": d.Kind, "listKind": d.Kind + "List", "plural": d.Plural,
		"singular": strings.ToLower(d.Kind), "categories": []string{"assnetes"},
	}
	if len(d.ShortNames) > 0 {
		names["shortNames"] = d.ShortNames
	}
	crd := map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata": map[string]any{
			"name":        d.Plural + "." + perevod.Group,
			"annotations": map[string]any{perevod.AnnName: d.Ru},
		},
		"spec": map[string]any{
			"group": perevod.Group,
			"names": names,
			"scope": scope,
			"versions": []any{map[string]any{
				"name": "v1", "served": true, "storage": true,
				"schema":                   map[string]any{"openAPIV3Schema": root},
				"subresources":             map[string]any{"status": map[string]any{}},
				"additionalPrinterColumns": cols,
			}},
		},
	}
	return json.MarshalIndent(crd, "", "  ")
}

func withRequired(s map[string]any, req []string) map[string]any {
	if len(req) > 0 {
		s["required"] = req
	}
	return s
}

// defaultOf brings a default to the type of its реквизит: YAML reads «1»
// as a number even where the реквизит is a Строка such as a CPU quantity.
func defaultOf(f Field) any {
	switch f.Type {
	case "Строка", "Дата":
		if _, ok := f.Default.(string); !ok {
			return fmt.Sprint(f.Default)
		}
	}
	return f.Default
}
