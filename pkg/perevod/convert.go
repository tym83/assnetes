package perevod

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tym83/assnetes/pkg/bsl"
)

// entries lists the keys and values of a Структура or Соответствие in order.
func entries(v bsl.Value) ([]string, []bsl.Value, bool) {
	if l, ok := v.(interface{ IsList() bool }); ok && l.IsList() {
		return nil, nil, false
	}
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
	ctx := &walk{kind: apiKind, labels: map[string]string{}, names: map[string]string{}}
	for _, k := range order {
		if k == "apiVersion" || k == "kind" {
			continue
		}
		v := top[k]
		switch {
		case k == "metadata":
			md, err := ctx.metadata(v)
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
	ctx.annotate(out)
	return out, nil
}

// annotate keeps in the top metadata what transliteration lost anywhere in
// the object, so the object reads back as it was written.
func (w *walk) annotate(obj *bsl.Map) {
	if len(w.labels) == 0 && len(w.names) == 0 {
		return
	}
	mdv, ok := obj.Lookup("metadata")
	md, isMap := mdv.(*bsl.Map)
	if !ok || !isMap {
		md = bsl.NewMap()
		obj.Insert("metadata", md)
	}
	av, _ := md.Lookup("annotations")
	anns, isMap := av.(*bsl.Map)
	if !isMap {
		anns = bsl.NewMap()
		md.Insert("annotations", anns)
	}
	if len(w.labels) > 0 {
		data, _ := json.Marshal(w.labels)
		anns.Insert(AnnLabels, string(data))
	}
	if len(w.names) > 0 {
		data, _ := json.Marshal(w.names)
		anns.Insert(AnnNames, string(data))
	}
}

type walk struct {
	kind   string
	labels map[string]string // API key → "ключ=значение" as written
	names  map[string]string // API name → as written
}

// value translates a value found under field (API name) at path.
func (w *walk) value(path, field string, v bsl.Value) bsl.Value {
	if field == "metadata" && isObject(v) {
		if md, err := w.metadata(v); err == nil {
			return md
		}
	}
	if els, ok := elements(v); ok {
		arr := bsl.NewArray()
		for _, e := range els {
			arr.Elems = append(arr.Elems, w.value(path+"[]", field, e))
		}
		return arr
	}
	if !isObject(v) {
		if s, ok := v.(string); ok && isNameField(field) && !IsAPIName(s) && s != "" {
			w.names[Name(s)] = s
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
			key := k
			if field != "data" && field != "stringData" && field != "binaryData" {
				key = AnnotationKey(k)
			} else if !isConfigKey(k) {
				key = configKey(k)
				w.names[key] = k
			}
			if s, ok := val.(string); ok && (field == "matchLabels" || field == "nodeSelector" || field == "selector") {
				val = LabelValue(s)
				if key != k || val != s {
					w.labels[key] = k + "=" + s
				}
			}
			out.Insert(key, plain(val))
			continue
		}
		en := FieldToAPI(k)
		out.Insert(en, w.value(path+"."+en, en, vals[i]))
	}
	return out
}

// isConfigKey reports whether k can be a key of the data of a ConfigMap or
// a Secret as it is.
func isConfigKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

// configKey transliterates a data key, keeping its extension: «настройки.bsl»
// becomes «nastrojki.bsl».
func configKey(k string) string {
	base, ext := k, ""
	if i := strings.LastIndex(k, "."); i > 0 && isConfigKey(k[i:]) {
		base, ext = k[:i], k[i:]
	}
	return Translit(base) + ext
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

func (w *walk) metadata(v bsl.Value) (*bsl.Map, error) {
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
					w.labels[apiKey] = key + "=" + val
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
	r := &reverse{labels: map[string][2]string{}, names: map[string]string{}}
	r.collect(obj)
	return r.fromAPI(obj)
}

// reverse restores what transliteration lost, from the annotations the
// object itself carries.
type reverse struct {
	labels map[string][2]string // "ключ=значение" in the API → as written
	names  map[string]string
}

func (r *reverse) collect(v bsl.Value) {
	if els, ok := elements(v); ok {
		for _, e := range els {
			r.collect(e)
		}
		return
	}
	keys, vals, ok := entries(v)
	if !ok {
		return
	}
	for i, k := range keys {
		if k == AnnNames {
			_ = json.Unmarshal([]byte(bsl.ToString(vals[i])), &r.names)
			continue
		}
		if k == AnnLabels {
			m := map[string]string{}
			if json.Unmarshal([]byte(bsl.ToString(vals[i])), &m) == nil {
				for apiKey, orig := range m {
					rk, rv, _ := strings.Cut(orig, "=")
					r.labels[apiKey+"="+LabelValue(rv)] = [2]string{rk, rv}
					r.labels[apiKey+"="] = [2]string{rk, ""}
				}
			}
			continue
		}
		r.collect(vals[i])
	}
}

func (r *reverse) label(k string, v bsl.Value) (string, bsl.Value) {
	if orig, ok := r.labels[k+"="+bsl.ToString(v)]; ok {
		return orig[0], orig[1]
	}
	if orig, ok := r.labels[k+"="]; ok {
		return orig[0], v
	}
	return AnnotationKeyRu(k), v
}

func (r *reverse) fromAPI(obj bsl.Value) (*bsl.Map, error) {
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
			out.Insert(FieldToRu(k), r.metadataFromAPI(v))
		case "spec", "status":
			if native {
				out.Insert(FieldToRu(k), plain(v))
			} else {
				out.Insert(FieldToRu(k), r.value(kindName, k, k, v))
			}
		default:
			out.Insert(FieldToRu(k), r.value(kindName, k, k, v))
		}
	}
	return out, nil
}

func (r *reverse) value(kind, path, field string, v bsl.Value) bsl.Value {
	if field == "metadata" && isObject(v) {
		return r.metadataFromAPI(v)
	}
	if els, ok := elements(v); ok {
		arr := bsl.NewArray()
		for _, e := range els {
			arr.Elems = append(arr.Elems, r.value(kind, path+"[]", field, e))
		}
		return arr
	}
	keys, vals, ok := entries(v)
	if !ok {
		if s, isStr := v.(string); isStr && isNameField(field) {
			if orig, found := r.names[s]; found {
				return orig
			}
		}
		return v
	}
	out := bsl.NewMap()
	labelLike := field == "matchLabels" || field == "nodeSelector" || kind == "Service" && path == "spec.selector"
	for i, k := range keys {
		switch {
		case labelLike:
			rk, rv := r.label(k, vals[i])
			out.Insert(rk, rv)
		case field == "data" || field == "stringData" || field == "binaryData":
			key := k
			if orig, ok := r.names[k]; ok {
				key = orig
			}
			out.Insert(key, plain(vals[i]))
		case freeFormFields[field]:
			out.Insert(AnnotationKeyRu(k), plain(vals[i]))
		default:
			out.Insert(FieldToRu(k), r.value(kind, path+"."+k, k, vals[i]))
		}
	}
	return out
}

func (r *reverse) metadataFromAPI(v bsl.Value) bsl.Value {
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
				if ak[j] == AnnName || ak[j] == AnnLabels || ak[j] == AnnNames || ak[j] == "kubectl.kubernetes.io/last-applied-configuration" {
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
			out.Insert(FieldToRu(k), r.value("", k, k, vals[i]))
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
