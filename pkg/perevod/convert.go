package perevod

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tym83/assnetes/pkg/bsl"
)

// entries lists the keys and values of a Структура or Соответствие in order.
func entries(v bsl.Value) ([]string, []bsl.Value, bool) {
	switch x := v.(type) {
	case *bsl.Structure:
		keys := x.Keys()
		vals := make([]bsl.Value, len(keys))
		for i, k := range keys {
			vals[i], _ = x.Get(k)
		}
		return keys, vals, true
	case *bsl.Map:
		var keys []string
		var vals []bsl.Value
		for _, k := range x.Keys() {
			ks, ok := k.(string)
			if !ok {
				ks = bsl.ToString(k)
			}
			val, _ := x.Lookup(k)
			keys = append(keys, ks)
			vals = append(vals, val)
		}
		return keys, vals, true
	}
	if o, ok := v.(interface {
		Entries() ([]string, []bsl.Value)
	}); ok {
		k, val := o.Entries()
		return k, val, true
	}
	return nil, nil, false
}

func isObject(v bsl.Value) bool {
	_, _, ok := entries(v)
	return ok
}

func elements(v bsl.Value) ([]bsl.Value, bool) {
	if it, ok := v.(bsl.Iterable); ok {
		if _, _, obj := entries(v); !obj {
			return it.Items(), true
		}
	}
	return nil, false
}

func lookup(v bsl.Value, key string) (bsl.Value, bool) {
	keys, vals, ok := entries(v)
	if !ok {
		return nil, false
	}
	for i, k := range keys {
		if k == key {
			return vals[i], true
		}
	}
	return nil, false
}

// ToAPI translates an object written in Russian, or in English, or mixed,
// into the object the API takes.
func ToAPI(obj bsl.Value) (*bsl.Map, error) {
	keys, vals, ok := entries(obj)
	if !ok {
		return nil, fmt.Errorf("объект кластера должен быть Структурой или Соответствием, а не %s", bsl.TypeOf(obj))
	}
	top := map[string]bsl.Value{}
	var order []string
	for i, k := range keys {
		en := FieldToAPI(k)
		if _, dup := top[en]; !dup {
			order = append(order, en)
		}
		top[en] = vals[i]
	}
	kindName := bsl.ToString(top["kind"])
	if kindName == "" {
		return nil, fmt.Errorf("не указан Вид объекта. Без первички не проведу")
	}
	kind, known := LookupKind(kindName)
	out := bsl.NewMap()
	apiVersion := bsl.ToString(top["apiVersion"])
	switch {
	case apiVersion != "":
		apiVersion = GroupToAPI(apiVersion)
	case known:
		apiVersion = kind.APIVersion
	default:
		return nil, fmt.Errorf("вид «%s» нам не знаком: укажите ВерсияAPI", kindName)
	}
	out.Insert("apiVersion", apiVersion)
	apiKind := kindName
	if known {
		apiKind = kind.Kind
	}
	out.Insert("kind", apiKind)
	native := known && kind.Native
	ctx := &walk{kind: apiKind}
	for _, k := range order {
		if k == "apiVersion" || k == "kind" {
			continue
		}
		v := top[k]
		switch {
		case k == "metadata":
			md, err := metadataToAPI(v)
			if err != nil {
				return nil, err
			}
			out.Insert(k, md)
		case native && (k == "spec" || k == "status"):
			out.Insert(k, plain(v))
		default:
			out.Insert(k, ctx.value(k, k, v))
		}
	}
	return out, nil
}

type walk struct{ kind string }

// value translates a value found under field (API name) at path.
func (w *walk) value(path, field string, v bsl.Value) bsl.Value {
	if els, ok := elements(v); ok {
		arr := bsl.NewArray()
		for _, e := range els {
			arr.Elems = append(arr.Elems, w.value(path+"[]", field, e))
		}
		return arr
	}
	if !isObject(v) {
		if s, ok := v.(string); ok && isNameField(field) && !IsAPIName(s) && s != "" {
			return Name(s)
		}
		return v
	}
	keys, vals, _ := entries(v)
	out := bsl.NewMap()
	free := freeFormFields[field] || w.kind == "Service" && path == "spec.selector"
	for i, k := range keys {
		if free {
			val := vals[i]
			if s, ok := val.(string); ok && (field == "matchLabels" || field == "nodeSelector" || field == "selector") {
				val = LabelValue(s)
			}
			key := k
			if field != "data" && field != "stringData" && field != "binaryData" {
				key = AnnotationKey(k)
			}
			out.Insert(key, plain(val))
			continue
		}
		en := FieldToAPI(k)
		out.Insert(en, w.value(path+"."+en, en, vals[i]))
	}
	return out
}

func isNameField(field string) bool {
	if field == "name" || field == "namespace" {
		return true
	}
	return strings.HasSuffix(field, "Name") && field != "pathType"
}

// plain copies a value into Соответствия and Массивы without translating.
func plain(v bsl.Value) bsl.Value {
	if els, ok := elements(v); ok {
		arr := bsl.NewArray()
		for _, e := range els {
			arr.Elems = append(arr.Elems, plain(e))
		}
		return arr
	}
	keys, vals, ok := entries(v)
	if !ok {
		return v
	}
	out := bsl.NewMap()
	for i, k := range keys {
		out.Insert(k, plain(vals[i]))
	}
	return out
}

func metadataToAPI(v bsl.Value) (*bsl.Map, error) {
	keys, vals, ok := entries(v)
	if !ok {
		return nil, fmt.Errorf("Метаданные должны быть Структурой или Соответствием")
	}
	out := bsl.NewMap()
	annotations := bsl.NewMap()
	labelsRu := map[string]string{}
	var labels *bsl.Map
	for i, k := range keys {
		en := FieldToAPI(k)
		switch en {
		case "name":
			name := bsl.ToString(vals[i])
			out.Insert("name", Name(name))
			if !IsAPIName(name) {
				annotations.Insert(AnnName, name)
			}
		case "namespace":
			out.Insert("namespace", Name(bsl.ToString(vals[i])))
		case "labels":
			labels = bsl.NewMap()
			lk, lv, _ := entries(vals[i])
			for j, key := range lk {
				val := bsl.ToString(lv[j])
				apiKey, apiVal := AnnotationKey(key), LabelValue(val)
				labels.Insert(apiKey, apiVal)
				if apiKey != key || apiVal != val {
					labelsRu[apiKey] = key + "=" + val
				}
			}
		case "annotations":
			ak, av, _ := entries(vals[i])
			for j, key := range ak {
				annotations.Insert(AnnotationKey(key), bsl.ToString(av[j]))
			}
		default:
			out.Insert(en, plain(vals[i]))
		}
	}
	if labels != nil {
		out.Insert("labels", labels)
	}
	if len(labelsRu) > 0 {
		data, _ := json.Marshal(labelsRu)
		annotations.Insert(AnnLabels, string(data))
	}
	if annotations.Count() > 0 {
		out.Insert("annotations", annotations)
	}
	return out, nil
}

// FromAPI translates an object of the API for people: Russian fields,
// kinds and names as they were written.
func FromAPI(obj bsl.Value) (*bsl.Map, error) {
	keys, vals, ok := entries(obj)
	if !ok {
		return nil, fmt.Errorf("ожидается объект кластера")
	}
	kindName := ""
	if v, ok := lookup(obj, "kind"); ok {
		kindName = bsl.ToString(v)
	}
	kind, known := LookupKind(kindName)
	native := known && kind.Native
	out := bsl.NewMap()
	for i, k := range keys {
		v := vals[i]
		switch k {
		case "apiVersion":
			out.Insert(FieldToRu(k), GroupToRu(bsl.ToString(v)))
		case "kind":
			if known {
				out.Insert(FieldToRu(k), kind.Ru)
			} else {
				out.Insert(FieldToRu(k), v)
			}
		case "metadata":
			out.Insert(FieldToRu(k), metadataFromAPI(v))
		case "spec", "status":
			if native {
				out.Insert(FieldToRu(k), plain(v))
			} else {
				out.Insert(FieldToRu(k), fromAPIValue(k, v))
			}
		default:
			out.Insert(FieldToRu(k), fromAPIValue(k, v))
		}
	}
	return out, nil
}

func fromAPIValue(field string, v bsl.Value) bsl.Value {
	if els, ok := elements(v); ok {
		arr := bsl.NewArray()
		for _, e := range els {
			arr.Elems = append(arr.Elems, fromAPIValue(field, e))
		}
		return arr
	}
	keys, vals, ok := entries(v)
	if !ok {
		return v
	}
	out := bsl.NewMap()
	for i, k := range keys {
		if freeFormFields[field] {
			out.Insert(AnnotationKeyRu(k), plain(vals[i]))
			continue
		}
		out.Insert(FieldToRu(k), fromAPIValue(k, vals[i]))
	}
	return out
}

func metadataFromAPI(v bsl.Value) bsl.Value {
	keys, vals, ok := entries(v)
	if !ok {
		return v
	}
	var ann bsl.Value
	if a, ok := lookup(v, "annotations"); ok {
		ann = a
	}
	annStr := func(key string) string {
		if ann == nil {
			return ""
		}
		x, ok := lookup(ann, key)
		if !ok {
			return ""
		}
		return bsl.ToString(x)
	}
	labelsRu := map[string]string{}
	if s := annStr(AnnLabels); s != "" {
		_ = json.Unmarshal([]byte(s), &labelsRu)
	}
	out := bsl.NewMap()
	for i, k := range keys {
		switch k {
		case "name":
			name := bsl.ToString(vals[i])
			if orig := annStr(AnnName); orig != "" && Translit(orig) == name {
				name = orig
			}
			out.Insert(FieldToRu(k), name)
		case "labels":
			labels := bsl.NewMap()
			lk, lv, _ := entries(vals[i])
			for j, key := range lk {
				if orig, ok := labelsRu[key]; ok {
					rk, rv, _ := strings.Cut(orig, "=")
					labels.Insert(rk, rv)
					continue
				}
				labels.Insert(AnnotationKeyRu(key), lv[j])
			}
			out.Insert(FieldToRu(k), labels)
		case "annotations":
			anns := bsl.NewMap()
			ak, av, _ := entries(vals[i])
			for j := range ak {
				if ak[j] == AnnName || ak[j] == AnnLabels || ak[j] == "kubectl.kubernetes.io/last-applied-configuration" {
					continue
				}
				anns.Insert(AnnotationKeyRu(ak[j]), av[j])
			}
			if anns.Count() > 0 {
				out.Insert(FieldToRu(k), anns)
			}
		case "managedFields":
			// служебные записи: бухгалтерии не интересно, кто и каким полем владел
		default:
			out.Insert(FieldToRu(k), fromAPIValue(k, vals[i]))
		}
	}
	return out
}

// DisplayName returns the name of an API object as it was written.
func DisplayName(obj bsl.Value) string {
	md, ok := lookup(obj, "metadata")
	if !ok {
		return ""
	}
	n, _ := lookup(md, "name")
	name := bsl.ToString(n)
	if ann, ok := lookup(md, "annotations"); ok {
		if orig, ok := lookup(ann, AnnName); ok && Translit(bsl.ToString(orig)) == name {
			return bsl.ToString(orig)
		}
	}
	return name
}
