package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/golos"
	"github.com/tym83/assnetes/pkg/manifest"
	"github.com/tym83/assnetes/pkg/perevod"
)

var metaGet = metav1.GetOptions{}

const fieldManager = "1sctl"

func newApplyCommand(o *options) *cobra.Command {
	var files []string
	var params []string
	var dryRun bool
	cmd := &cobra.Command{
		Use:     "провести -ф ФАЙЛ...",
		Aliases: []string{"apply"},
		Short:   "провести объекты из манифестов на языке 1С (.bsl) или из YAML",
		Example: "  1сctl провести --файл бухгалтерия.bsl --параметр Реплики=2",
		RunE: func(cmd *cobra.Command, args []string) error {
			files = append(files, args...)
			if len(files) == 0 {
				return fmt.Errorf("какой документ проводить? Укажите --файл")
			}
			p := map[string]string{}
			for _, kv := range params {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					return fmt.Errorf("параметр «%s»: ожидается Имя=Значение", kv)
				}
				p[k] = v
			}
			var objs []*bsl.Map
			for _, f := range files {
				got, err := o.load(cmd.Context(), f, p)
				if err != nil {
					return err
				}
				objs = append(objs, got...)
			}
			if dryRun {
				return o.printObjects(objs, "yaml")
			}
			return o.apply(cmd.Context(), objs)
		},
	}
	cmd.Flags().StringArrayVar(&files, "файл", nil, "манифест: .bsl, .yaml или .json")
	cmd.Flags().StringArrayVarP(&files, "filename", "f", nil, "")
	_ = cmd.Flags().MarkHidden("filename")
	cmd.Flags().StringArrayVar(&params, "параметр", nil, "Имя=Значение для Параметры в манифесте")
	cmd.Flags().BoolVar(&dryRun, "пробно", false, "показать объекты для API, ничего не проводя")
	return cmd
}

// load reads a manifest file into objects of the API.
func (o *options) load(ctx context.Context, path string, params map[string]string) ([]*bsl.Map, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("документ не найден: %w", err)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".bsl", ".os", ".1с", ".1c":
		res, err := manifest.Run(ctx, filepath.Base(path), string(data), manifest.Options{
			Params:  params,
			Message: func(s string) { fmt.Fprintln(o.errOut, s) },
		})
		if err != nil {
			return nil, err
		}
		return res.Objects, nil
	}
	return loadYAML(data)
}

// loadYAML reads YAML or JSON documents, in Russian or in English.
func loadYAML(data []byte) ([]*bsl.Map, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var out []*bsl.Map
	for {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("ошибка разбора YAML: %w", err)
		}
		v, err := yamlToValue(&n)
		if err != nil {
			return nil, err
		}
		if v == nil {
			continue
		}
		obj, err := perevod.ToAPI(v)
		if err != nil {
			return nil, err
		}
		out = append(out, obj)
	}
}

func yamlToValue(n *yaml.Node) (bsl.Value, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return yamlToValue(n.Content[0])
	case yaml.MappingNode:
		m := bsl.NewMap()
		for i := 0; i+1 < len(n.Content); i += 2 {
			v, err := yamlToValue(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m.Insert(n.Content[i].Value, v)
		}
		return m, nil
	case yaml.SequenceNode:
		a := bsl.NewArray()
		for _, c := range n.Content {
			v, err := yamlToValue(c)
			if err != nil {
				return nil, err
			}
			a.Elems = append(a.Elems, v)
		}
		return a, nil
	case yaml.AliasNode:
		return yamlToValue(n.Alias)
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!int", "!!float":
			return bsl.ParseNumber(n.Value)
		case "!!bool":
			return n.Value == "true", nil
		case "!!null":
			return bsl.Undefined{}, nil
		}
		return n.Value, nil
	}
	return nil, fmt.Errorf("неизвестный узел YAML")
}

func (o *options) apply(ctx context.Context, objs []*bsl.Map) error {
	c, err := o.client()
	if err != nil {
		return err
	}
	failed := 0
	for _, obj := range objs {
		if err := o.applyOne(ctx, c, obj); err != nil {
			failed++
			o.reportError(err, obj)
		}
	}
	o.calendar()
	if failed > 0 {
		return fmt.Errorf("не проведено документов: %d из %d", failed, len(objs))
	}
	return nil
}

func (o *options) applyOne(ctx context.Context, c *clients, obj *bsl.Map) error {
	u, err := toUnstructured(obj)
	if err != nil {
		return err
	}
	m, kind, err := c.resolveKind(u.GetKind())
	if err != nil {
		return err
	}
	if m.Scope.Name() == meta.RESTScopeNameNamespace && u.GetNamespace() == "" {
		u.SetNamespace(o.ns(c))
	}
	ri := c.resource(m, u.GetNamespace())
	before, getErr := ri.Get(ctx, u.GetName(), metaGet)
	if getErr != nil && !apierrors.IsNotFound(getErr) {
		return getErr
	}
	created := getErr != nil
	after, err := ri.Apply(ctx, u.GetName(), u, metav1.ApplyOptions{FieldManager: fieldManager, Force: true})
	if err != nil {
		return err
	}
	kindRu := u.GetKind()
	if kind != nil {
		kindRu = kind.Ru
	}
	ruObj, _ := fromUnstructured(after)
	name := perevod.DisplayName(ruObj)
	v := golos.Vars{"Вид": kindRu, "Имя": name, "Подразделение": u.GetNamespace()}
	switch {
	case created:
		fmt.Fprintf(o.out, "%s %s «%s» %s\n", green(o.tty, "✓"), kindRu, name, agree(kind, "проведён", "проведена", "проведено", "проведены"))
		o.say("proveden", v)
	case before.GetResourceVersion() == after.GetResourceVersion():
		fmt.Fprintf(o.out, "%s %s «%s» без изменений\n", green(o.tty, "="), kindRu, name)
		o.say("bez-izmenenij", v)
	default:
		fmt.Fprintf(o.out, "%s %s «%s» %s\n", green(o.tty, "✓"), kindRu, name, agree(kind, "перепроведён", "перепроведена", "перепроведено", "перепроведены"))
		o.say("perepoveden", v)
	}
	return nil
}

// reportError prints an error of the API worded the way 1C would word it,
// with the original below it.
func (o *options) reportError(err error, obj *bsl.Map) {
	v := golos.Vars{}
	if obj != nil {
		if ru, e := perevod.FromAPI(obj); e == nil {
			if k, ok := ru.Lookup("Вид"); ok {
				v["Вид"] = bsl.ToString(k)
			}
		}
		v["Имя"] = perevod.DisplayName(obj)
	}
	o.reportErrorVars(err, v)
}

func (o *options) reportErrorVars(err error, v golos.Vars) {
	fmt.Fprintln(o.errOut, red(isTTY(o.errOut), "✗ ")+o.golos.Error(err, v))
	fmt.Fprintln(o.errOut, dim(isTTY(o.errOut), "  "+err.Error()))
}

func agree(k *perevod.Kind, m, f, n, p string) string {
	if k == nil {
		return m
	}
	return k.Agree(m, f, n, p)
}
