package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/golos"
	"github.com/tym83/assnetes/pkg/perevod"
)

var phases = map[string]string{
	"Running": "Работает", "Pending": "Ожидает", "Succeeded": "Выполнен",
	"Failed": "Ошибка", "Unknown": "Неизвестно",
}

func newGetCommand(o *options) *cobra.Command {
	var selector string
	cmd := &cobra.Command{
		Use:     "получить ВИД [ИМЯ...]",
		Aliases: []string{"get"},
		Short:   "получить список объектов или объект",
		Example: "  1сctl получить поды\n  1сctl получить развёртывание бухгалтерия --вывод bsl",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.get(cmd.Context(), args[0], args[1:], selector)
		},
	}
	cmd.Flags().StringVar(&selector, "отбор", "", "отбор по меткам: приложение=бухгалтерия")
	cmd.Flags().StringVarP(&selector, "selector", "l", "", "")
	_ = cmd.Flags().MarkHidden("selector")
	return cmd
}

func (o *options) get(ctx context.Context, kindName string, names []string, selector string) error {
	c, err := o.client()
	if err != nil {
		return err
	}
	m, kind, err := c.resolveKind(kindName)
	if err != nil {
		return err
	}
	ns := o.ns(c)
	if o.allNS {
		ns = ""
	}
	var items []unstructured.Unstructured
	if len(names) > 0 {
		for _, n := range names {
			u, err := c.get(ctx, m, ns, perevod.Name(n))
			if err != nil {
				kr := m.GroupVersionKind.Kind
				if kind != nil {
					kr = kind.Ru
				}
				o.reportErrorVars(err, golos.Vars{"Вид": kr, "Имя": n})
				return fmt.Errorf("не найдено")
			}
			items = append(items, *u)
		}
	} else {
		sel := ""
		if selector != "" {
			sel = translateSelector(selector)
		}
		list, err := c.resource(m, ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
		if err != nil {
			o.reportError(err, nil)
			return fmt.Errorf("список не получен")
		}
		items = list.Items
	}
	kindRu := m.GroupVersionKind.Kind
	if kind != nil {
		kindRu = kind.Plural
	}
	if o.output != "" && o.output != "таблица" {
		var objs []*bsl.Map
		for i := range items {
			v, err := fromUnstructured(&items[i])
			if err != nil {
				return err
			}
			objs = append(objs, v.(*bsl.Map))
		}
		return o.printObjects(objs, o.output)
	}
	if len(items) == 0 {
		fmt.Fprintf(o.out, "%s: нет\n", kindRu)
		o.say("pusto", golos.Vars{"Вид": kindRu})
		return nil
	}
	namespaced := m.Scope.Name() == meta.RESTScopeNameNamespace
	header, rows := tableFor(m.GroupVersionKind.Kind, items, namespaced && o.allNS)
	o.table(header, rows)
	o.calendar()
	return nil
}

// translateSelector turns «приложение=бухгалтерия» into the API form.
func translateSelector(s string) string {
	var parts []string
	for _, p := range strings.Split(s, ",") {
		for _, op := range []string{"!=", "==", "="} {
			if k, v, ok := strings.Cut(p, op); ok {
				p = perevod.AnnotationKey(strings.TrimSpace(k)) + op + perevod.LabelValue(strings.TrimSpace(v))
				break
			}
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ",")
}

func displayName(u *unstructured.Unstructured) string {
	if orig := u.GetAnnotations()[perevod.AnnName]; orig != "" && perevod.Translit(orig) == u.GetName() {
		return orig
	}
	return u.GetName()
}

func displayNS(u *unstructured.Unstructured) string { return u.GetNamespace() }

func tableFor(kind string, items []unstructured.Unstructured, withNS bool) ([]string, [][]string) {
	header := []string{"ИМЯ"}
	if withNS {
		header = []string{"ПОДРАЗДЕЛЕНИЕ", "ИМЯ"}
	}
	var extra []string
	switch kind {
	case "Pod":
		extra = []string{"ГОТОВО", "ФАЗА", "ПЕРЕЗАПУСКИ"}
	case "Deployment", "StatefulSet", "ReplicaSet":
		extra = []string{"РЕПЛИКИ"}
	case "Node":
		extra = []string{"ВЕРСИЯ"}
	case "Service":
		extra = []string{"ТИП", "АДРЕС"}
	}
	header = append(header, extra...)
	header = append(header, "СОСТОЯНИЕ", "ВОЗРАСТ")
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].GetNamespace() != items[j].GetNamespace() {
			return items[i].GetNamespace() < items[j].GetNamespace()
		}
		return displayName(&items[i]) < displayName(&items[j])
	})
	var rows [][]string
	for i := range items {
		u := &items[i]
		row := []string{displayName(u)}
		if withNS {
			row = []string{displayNS(u), displayName(u)}
		}
		state := "✓ проведён"
		switch kind {
		case "Pod":
			ready, total, restarts := podCounts(u)
			phase, _, _ := unstructured.NestedString(u.Object, "status", "phase")
			row = append(row, fmt.Sprintf("%d/%d", ready, total), phases[phase], fmt.Sprint(restarts))
			switch {
			case phase == "Pending":
				state = "○ записан, не проведён"
			case phase == "Failed":
				state = "! ошибка проведения"
			case phase == "Running" && ready < total:
				state = "○ проводится"
			}
		case "Deployment", "StatefulSet", "ReplicaSet":
			want, _, _ := unstructured.NestedInt64(u.Object, "spec", "replicas")
			ready, _, _ := unstructured.NestedInt64(u.Object, "status", "readyReplicas")
			row = append(row, fmt.Sprintf("%d/%d", ready, want))
			if ready < want {
				state = "○ проводится"
			}
		case "Node":
			v, _, _ := unstructured.NestedString(u.Object, "status", "nodeInfo", "kubeletVersion")
			row = append(row, v)
			if !nodeReady(u) {
				state = "! склад закрыт"
			}
		case "Service":
			t, _, _ := unstructured.NestedString(u.Object, "spec", "type")
			ip, _, _ := unstructured.NestedString(u.Object, "spec", "clusterIP")
			row = append(row, t, ip)
		}
		if u.GetDeletionTimestamp() != nil {
			state = "✗ помечен на удаление"
		}
		row = append(row, state, age(u.GetCreationTimestamp().Time))
		rows = append(rows, row)
	}
	return header, rows
}

func podCounts(u *unstructured.Unstructured) (ready, total int, restarts int64) {
	statuses, _, _ := unstructured.NestedSlice(u.Object, "status", "containerStatuses")
	containers, _, _ := unstructured.NestedSlice(u.Object, "spec", "containers")
	total = len(containers)
	for _, s := range statuses {
		m, _ := s.(map[string]any)
		if r, _ := m["ready"].(bool); r {
			ready++
		}
		switch n := m["restartCount"].(type) {
		case int64:
			restarts += n
		case float64:
			restarts += int64(n)
		}
	}
	return
}

func nodeReady(u *unstructured.Unstructured) bool {
	conds, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, c := range conds {
		m, _ := c.(map[string]any)
		if m["type"] == "Ready" {
			return m["status"] == "True"
		}
	}
	return false
}

func newDescribeCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:     "описать ВИД ИМЯ",
		Aliases: []string{"describe"},
		Short:   "показать объект целиком, по-русски",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.output == "" {
				o.output = "yaml"
			}
			return o.get(cmd.Context(), args[0], args[1:], "")
		},
	}
}

func newExportCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:     "выгрузить ВИД ИМЯ",
		Aliases: []string{"export"},
		Short:   "выгрузить живой объект в манифест на языке 1С",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.output = "bsl"
			return o.get(cmd.Context(), args[0], args[1:], "")
		},
	}
}

func newDeleteCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:     "удалить ВИД ИМЯ...",
		Aliases: []string{"delete"},
		Short:   "удалить объекты сразу, без пометки",
		Args:    cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := o.client()
			if err != nil {
				return err
			}
			m, kind, err := c.resolveKind(args[0])
			if err != nil {
				return err
			}
			kindRu := m.GroupVersionKind.Kind
			if kind != nil {
				kindRu = kind.Ru
			}
			for _, n := range args[1:] {
				if err := c.resource(m, o.ns(c)).Delete(cmd.Context(), perevod.Name(n), metav1.DeleteOptions{}); err != nil {
					o.reportError(err, nil)
					return fmt.Errorf("не удалено")
				}
				fmt.Fprintf(o.out, "%s %s «%s» %s\n", red(o.tty, "✗"), kindRu, n, agree(kind, "удалён", "удалена", "удалено", "удалены"))
				o.say("udalen", golos.Vars{"Вид": kindRu, "Имя": n})
			}
			return nil
		},
	}
}

// AnnMarked is the mark the deletion processing looks for.
const AnnMarked = perevod.Group + "/pometka-udaleniya"

func newMarkCommand(o *options) *cobra.Command {
	var undo bool
	cmd := &cobra.Command{
		Use:     "пометить ВИД ИМЯ...",
		Aliases: []string{"распровести", "mark"},
		Short:   "пометить на удаление: удалит обработка «Удаление помеченных», если на объект нет ссылок",
		Args:    cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := o.client()
			if err != nil {
				return err
			}
			m, kind, err := c.resolveKind(args[0])
			if err != nil {
				return err
			}
			kindRu := m.GroupVersionKind.Kind
			if kind != nil {
				kindRu = kind.Ru
			}
			value := `"` + time.Now().UTC().Format(time.RFC3339) + `"`
			if undo {
				value = "null"
			}
			patch := []byte(`{"metadata":{"annotations":{"` + AnnMarked + `":` + value + `}}}`)
			for _, n := range args[1:] {
				_, err := c.resource(m, o.ns(c)).Patch(cmd.Context(), perevod.Name(n), "application/merge-patch+json", patch, metav1.PatchOptions{FieldManager: fieldManager})
				if err != nil {
					o.reportError(err, nil)
					return fmt.Errorf("пометка не поставлена")
				}
				if undo {
					fmt.Fprintf(o.out, "%s %s «%s»: пометка на удаление снята\n", green(o.tty, "↺"), kindRu, n)
					continue
				}
				fmt.Fprintf(o.out, "%s %s «%s» %s на удаление\n", red(o.tty, "✗"), kindRu, n, agree(kind, "помечен", "помечена", "помечено", "помечены"))
				o.say("pomechen", golos.Vars{"Вид": kindRu, "Имя": n})
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&undo, "снять", false, "снять пометку на удаление")
	return cmd
}

func newLogsCommand(o *options) *cobra.Command {
	var container string
	var follow bool
	cmd := &cobra.Command{
		Use:     "журнал ПОД",
		Aliases: []string{"logs"},
		Short:   "журнал регистрации пода",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := o.client()
			if err != nil {
				return err
			}
			req := c.typed.CoreV1().Pods(o.ns(c)).GetLogs(perevod.Name(args[0]), &corev1.PodLogOptions{Container: perevod.Name(container), Follow: follow})
			if container == "" {
				req = c.typed.CoreV1().Pods(o.ns(c)).GetLogs(perevod.Name(args[0]), &corev1.PodLogOptions{Follow: follow})
			}
			rc, err := req.Stream(cmd.Context())
			if err != nil {
				o.reportError(err, nil)
				return fmt.Errorf("журнал регистрации недоступен")
			}
			defer rc.Close()
			_, err = io.Copy(os.Stdout, rc)
			return err
		},
	}
	cmd.Flags().StringVar(&container, "контейнер", "", "контейнер пода")
	cmd.Flags().BoolVar(&follow, "следить", false, "следить за журналом")
	return cmd
}

func newKindsCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:     "виды",
		Aliases: []string{"api-resources"},
		Short:   "план видов: какие объекты ведутся и как их называть",
		RunE: func(*cobra.Command, []string) error {
			var rows [][]string
			for _, k := range perevod.Kinds {
				rows = append(rows, []string{k.Ru, k.Plural, k.Kind, perevod.GroupToRu(k.APIVersion)})
			}
			o.table([]string{"ВИД", "МНОЖЕСТВЕННОЕ", "В API", "ВЕРСИЯ API"}, rows)
			return nil
		},
	}
}
