package cli

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/manifest"
	"github.com/tym83/assnetes/pkg/perevod"
)

// printObjects prints objects of the API in Russian: as YAML, JSON or the
// source of a manifest.
func (o *options) printObjects(objs []*bsl.Map, format string) error {
	for i, obj := range objs {
		if format == "api" {
			// what the API server stores, for tools that do not speak Russian
			if i > 0 {
				fmt.Fprintln(o.out, "---")
			}
			y, err := toYAML(obj)
			if err != nil {
				return err
			}
			fmt.Fprint(o.out, y)
			continue
		}
		ru, err := perevod.FromAPI(obj)
		if err != nil {
			return err
		}
		switch format {
		case "json":
			js, err := bsl.WriteJSON(ru, "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(o.out, js)
		case "bsl":
			src, err := manifest.Export(ru)
			if err != nil {
				return err
			}
			if i > 0 {
				fmt.Fprintln(o.out)
			}
			fmt.Fprint(o.out, src)
		default:
			if i > 0 {
				fmt.Fprintln(o.out, "---")
			}
			y, err := toYAML(ru)
			if err != nil {
				return err
			}
			fmt.Fprint(o.out, y)
		}
	}
	return nil
}

func toYAML(v bsl.Value) (string, error) {
	n, err := yamlNode(v)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return "", err
	}
	return b.String(), nil
}

func yamlNode(v bsl.Value) (*yaml.Node, error) {
	switch x := v.(type) {
	case *bsl.Map:
		n := &yaml.Node{Kind: yaml.MappingNode}
		for _, k := range x.Keys() {
			val, _ := x.Lookup(k)
			c, err := yamlNode(val)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: bsl.ToString(k)}, c)
		}
		return n, nil
	case *bsl.Array:
		n := &yaml.Node{Kind: yaml.SequenceNode}
		for _, e := range x.Elems {
			c, err := yamlNode(e)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, c)
		}
		return n, nil
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: x}, nil
	case bsl.Number:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: x.Plain()}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprint(x)}, nil
	case bsl.Undefined, nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Value: bsl.ToString(v)}, nil
}

// table prints rows aligned in columns. Widths count letters, not bytes.
func (o *options) table(header []string, rows [][]string) {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = len([]rune(h))
	}
	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], len([]rune(c)))
		}
	}
	line := func(cells []string, style func(bool, string) string) {
		var b strings.Builder
		for i, c := range cells {
			cell := c
			if i < len(cells)-1 {
				cell += strings.Repeat(" ", widths[i]-len([]rune(c))+3)
			}
			b.WriteString(cell)
		}
		fmt.Fprintln(o.out, style(o.tty, strings.TrimRight(b.String(), " ")))
	}
	line(header, bold)
	for _, r := range rows {
		line(r, func(_ bool, s string) string { return s })
	}
}

// age writes how long ago something happened the way a person would.
func age(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d сек", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d мин", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d ч", int(d.Hours()))
	}
	return fmt.Sprintf("%d дн", int(d.Hours()/24))
}
