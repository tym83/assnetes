package cli

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/tym83/assnetes/pkg/bsl"
)

//go:embed vvod
var vvodFS embed.FS

// PriceList is the repository of packages of the distribution; Kuberoot is
// the one of kubepkg's recipes, where CloudNativePG for 1C comes from.
const (
	PriceList = "https://tym83.github.io/assnetes/index.yaml"
	Kuberoot  = "https://kuberoot-dev.github.io/kubepkg-recipes/index.yaml"
)

func newVvodCommand(o *options) *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:     "ввод-начальных-остатков",
		Aliases: []string{"bootstrap"},
		Short:   "открыть склад: поставить пакетный менеджер и подписаться на прайс-лист",
		Long: "Ставит в кластер пакетный менеджер (kubepkg под группой «пакеты.бух») и подписывает\n" +
			"кластер на прайс-лист 1Ж:Оркестратора 8. После этого: 1сctl прайс, 1сctl закупить битрикс.",
		RunE: func(cmd *cobra.Command, _ []string) error { return o.vvod(cmd.Context(), url) },
	}
	cmd.Flags().StringVar(&url, "прайс-лист", PriceList, "адрес индекса пакетов")
	return cmd
}

func (o *options) vvod(ctx context.Context, url string) error {
	c, err := o.client()
	if err != nil {
		return err
	}
	ns, err := loadYAML([]byte("apiVersion: v1\nkind: Namespace\nmetadata: {name: kubepkg-system}\n"))
	if err != nil {
		return err
	}
	if err := o.applyQuiet(ctx, c, ns); err != nil {
		return err
	}
	names, _ := fs.Glob(vvodFS, "vvod/*.yaml")
	sort.Strings(names) // the CRDs first, the operator last
	for _, n := range names {
		data, _ := vvodFS.ReadFile(n)
		objs, err := loadYAML(data)
		if err != nil {
			return fmt.Errorf("%s: %w", n, err)
		}
		if err := o.applyQuiet(ctx, c, objs); err != nil {
			return err
		}
	}
	fmt.Fprintln(o.out, green(o.tty, "✓")+" Пакетный менеджер поставлен")

	for _, r := range []struct{ name, url, key string }{
		{"prajs-list", url, "vvod/index.pub"},
		{"kuberoot", Kuberoot, "vvod/kuberoot.pub"},
	} {
		key, _ := vvodFS.ReadFile(r.key)
		repo := bsl.NewMap()
		repo.Insert("apiVersion", GroupPakety+"/v1")
		repo.Insert("kind", "Repository")
		md := bsl.NewMap()
		md.Insert("name", r.name)
		repo.Insert("metadata", md)
		spec := bsl.NewMap()
		spec.Insert("url", r.url)
		spec.Insert("interval", "10m")
		spec.Insert("publicKeys", bsl.NewArray(string(key)))
		repo.Insert("spec", spec)
		// the API server takes Repositories once the new kinds are served
		err = wait.PollUntilContextTimeout(ctx, 2*time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
			err := o.applyQuiet(ctx, c, []*bsl.Map{repo})
			if err != nil && strings.Contains(err.Error(), "не ведётся") {
				return false, nil
			}
			return err == nil, err
		})
		if err != nil {
			return fmt.Errorf("поставщик %s не подписан: %w", r.name, err)
		}
	}
	fmt.Fprintln(o.out, green(o.tty, "✓")+" Подписка на прайс-лист: "+url)
	fmt.Fprintln(o.out, green(o.tty, "✓")+" Подписка на поставщика kuberoot: "+Kuberoot)
	o.say("vvod-ostatkov", nil)
	return nil
}

// applyQuiet posts objects without a word per object.
func (o *options) applyQuiet(ctx context.Context, c *clients, objs []*bsl.Map) error {
	for _, obj := range objs {
		u, err := toUnstructured(obj)
		if err != nil {
			return err
		}
		m, _, err := c.mappingOf(u)
		if err != nil {
			return err
		}
		if _, err := c.resource(m, u.GetNamespace()).Apply(ctx, u.GetName(), u, applyOptions); err != nil {
			return fmt.Errorf("%s %s: %w", u.GetKind(), u.GetName(), err)
		}
	}
	return nil
}
