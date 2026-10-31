package cli

// «1сctl учёт»: БухОпс from the command line. Operations are posted with
// «1сctl провести» like any document; here they are listed, signed a
// second time, reversed by сторно, the period is closed, and the books
// are compared with the cluster.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/golos"
	"github.com/tym83/assnetes/pkg/perevod"
)

func newUchetCommand(o *options) *cobra.Command {
	var book string
	cmd := &cobra.Command{
		Use:     "учёт",
		Aliases: []string{"учет", "uchet", "bukhops"},
		Short:   "БухОпс: журнал операций, вторая подпись, сторно, закрытие периода, сверка и ОСВ",
	}
	cmd.PersistentFlags().StringVar(&book, "книга", "", "главная книга (по умолчанию — единственная в подразделении)")

	journal := &cobra.Command{
		Use:     "операции",
		Aliases: []string{"журнал-операций"},
		Short:   "журнал операций книги: проводки, подписи и учтено ли",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return o.uchetJournal(cmd.Context(), book)
		},
	}

	approve := &cobra.Command{
		Use:   "утвердить ОПЕРАЦИЯ...",
		Short: "поставить вторую подпись",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.uchetApprove(cmd.Context(), args)
		},
	}

	var date, content string
	storno := &cobra.Command{
		Use:   "сторно ОПЕРАЦИЯ",
		Short: "отменить учтённую операцию: те же проводки с минусом, первичка — прежняя",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.uchetStorno(cmd.Context(), args[0], date, content)
		},
	}
	storno.Flags().StringVar(&date, "дата", "", "дата сторно, ГГГГ-ММ-ДД (по умолчанию сегодня)")
	storno.Flags().StringVar(&content, "содержание", "", "почему сторнируем")

	closePeriod := &cobra.Command{
		Use:     "закрыть-период ДАТА",
		Short:   "дата запрета изменения данных: раньше неё операции не принимаются",
		Example: "  1сctl учёт закрыть-период 2026-10-31",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.uchetClose(cmd.Context(), book, args[0])
		},
	}

	sverka := &cobra.Command{
		Use:   "сверка",
		Short: "сверка кластера с учётом: ревизия, разнесено ли и расхождения",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return o.uchetSverka(cmd.Context(), book)
		},
	}

	osv := &cobra.Command{
		Use:     "осв",
		Aliases: []string{"оборотно-сальдовая-ведомость"},
		Short:   "оборотно-сальдовая ведомость книги за открытый период",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return o.uchetOSV(cmd.Context(), book)
		},
	}

	cmd.AddCommand(journal, approve, storno, closePeriod, sverka, osv)
	return cmd
}

// whoami is the user the cluster sees: the one who signs.
func (c *clients) whoami(ctx context.Context) (string, error) {
	r, err := c.typed.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("не понять, кто подписывает: %w", err)
	}
	return r.Status.UserInfo.Username, nil
}

// signAuthor fills Автор of operations with the user who posts them.
func (o *options) signAuthor(ctx context.Context, c *clients, u *unstructured.Unstructured) error {
	if u.GetAPIVersion() != perevod.Group+"/v1" || u.GetKind() != "Operation" {
		return nil
	}
	if a, _, _ := unstructured.NestedString(u.Object, "spec", "Автор"); a != "" {
		return nil
	}
	me, err := c.whoami(ctx)
	if err != nil {
		return err
	}
	return unstructured.SetNestedField(u.Object, me, "spec", "Автор")
}

func (o *options) uchetKind(c *clients, kind string) (meta.RESTMapping, error) {
	m, _, err := c.resolveKind(kind)
	if err != nil {
		return meta.RESTMapping{}, fmt.Errorf("БухОпс в этом кластере не ведётся: поставьте обработку «Главная книга» (%w)", err)
	}
	return *m, nil
}

// ledger finds the ledger to work with: the one named, or the only one.
func (o *options) ledger(ctx context.Context, c *clients, name string) (*unstructured.Unstructured, error) {
	m, err := o.uchetKind(c, "ГлавнаяКнига")
	if err != nil {
		return nil, err
	}
	ri := c.resource(&m, o.ns(c))
	if name != "" {
		u, err := ri.Get(ctx, perevod.Name(name), metaGet)
		if err != nil {
			return nil, fmt.Errorf("главная книга «%s» не найдена в подразделении %s", name, c.nsName(o.ns(c)))
		}
		return u, nil
	}
	l, err := ri.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	switch len(l.Items) {
	case 0:
		return nil, fmt.Errorf("в подразделении %s главной книги нет", c.nsName(o.ns(c)))
	case 1:
		return &l.Items[0], nil
	}
	var names []string
	for i := range l.Items {
		names = append(names, displayName(&l.Items[i]))
	}
	return nil, fmt.Errorf("книг несколько (%s): укажите --книга", strings.Join(names, ", "))
}

// operations of a ledger, in the order the books take them.
func (o *options) operations(ctx context.Context, c *clients, ledger *unstructured.Unstructured) ([]unstructured.Unstructured, error) {
	m, err := o.uchetKind(c, "Операция")
	if err != nil {
		return nil, err
	}
	l, err := c.resource(&m, ledger.GetNamespace()).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var out []unstructured.Unstructured
	for _, u := range l.Items {
		b, _, _ := unstructured.NestedString(u.Object, "spec", "Книга")
		if perevod.Name(b) == ledger.GetName() {
			out = append(out, u)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := opDate(&out[i]), opDate(&out[j])
		if di != dj {
			return di < dj
		}
		ti, tj := out[i].GetCreationTimestamp(), out[j].GetCreationTimestamp()
		if !ti.Equal(&tj) {
			return ti.Before(&tj)
		}
		return out[i].GetName() < out[j].GetName()
	})
	return out, nil
}

func opDate(u *unstructured.Unstructured) string {
	d, _, _ := unstructured.NestedString(u.Object, "spec", "Дата")
	if len(d) > 10 {
		d = d[:10]
	}
	return d
}

// number prints an amount the way a balance sheet does: blank for zero.
func number(v any) string {
	switch n := v.(type) {
	case int64:
		if n == 0 {
			return ""
		}
		return fmt.Sprint(n)
	case float64:
		if n == 0 {
			return ""
		}
		return fmt.Sprint(n)
	case bsl.Number:
		if n.Sign() == 0 {
			return ""
		}
		return n.String()
	}
	return fmt.Sprint(v)
}

func (o *options) uchetJournal(ctx context.Context, name string) error {
	c, err := o.client()
	if err != nil {
		return err
	}
	ledger, err := o.ledger(ctx, c, name)
	if err != nil {
		return err
	}
	ops, err := o.operations(ctx, c, ledger)
	if err != nil {
		return err
	}
	if len(ops) == 0 {
		fmt.Fprintf(o.out, "В книге «%s» операций нет.\n", displayName(ledger))
		return nil
	}
	header := []string{"ОПЕРАЦИЯ", "ДАТА", "СОДЕРЖАНИЕ", "ДТ", "КТ", "СУММА", "АВТОР", "УТВЕРДИЛ", "УЧЁТ"}
	var rows [][]string
	for i := range ops {
		u := &ops[i]
		spec, _, _ := unstructured.NestedMap(u.Object, "spec")
		str := func(k string) string { s, _ := spec[k].(string); return s }
		state := opState(u)
		entries, _, _ := unstructured.NestedSlice(u.Object, "spec", "Проводки")
		first := []string{displayName(u), opDate(u), str("Содержание")}
		tail := []string{str("Автор"), str("Утвердил"), state}
		if len(entries) == 0 {
			what := "—"
			switch {
			case str("Первичка") != "":
				what = "первичка"
			case str("Сторнирует") != "":
				what = "сторно первички"
			}
			rows = append(rows, append(append(first, "", "", what), tail...))
			continue
		}
		for j, e := range entries {
			m, _ := e.(map[string]any)
			line := []string{fmt.Sprint(m["Дт"]), fmt.Sprint(m["Кт"]), fmt.Sprint(m["Сумма"])}
			if j == 0 {
				if str("Первичка") != "" {
					line[2] += " +первичка"
				}
				rows = append(rows, append(append(append([]string{}, first...), line...), tail...))
			} else {
				rows = append(rows, append(append([]string{"", "", ""}, line...), "", "", ""))
			}
		}
	}
	fmt.Fprintln(o.out, bold(o.tty, "Журнал операций книги «"+displayName(ledger)+"»"))
	o.table(header, rows)
	return nil
}

func opState(u *unstructured.Unstructured) string {
	taken, found, _ := unstructured.NestedBool(u.Object, "status", "Учтена")
	reason, _, _ := unstructured.NestedString(u.Object, "status", "Причина")
	by, _, _ := unstructured.NestedString(u.Object, "status", "СторнированаОперацией")
	switch {
	case !found:
		return "…"
	case taken && by != "":
		return "сторнирована «" + by + "»"
	case taken:
		return "✓ учтена"
	case reason == "ждёт второй подписи":
		return "… " + reason
	}
	return "✗ " + reason
}

func (o *options) uchetApprove(ctx context.Context, names []string) error {
	c, err := o.client()
	if err != nil {
		return err
	}
	me, err := c.whoami(ctx)
	if err != nil {
		return err
	}
	m, err := o.uchetKind(c, "Операция")
	if err != nil {
		return err
	}
	patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"Утвердил": me}})
	failed := 0
	for _, n := range names {
		_, err := c.resource(&m, o.ns(c)).Patch(ctx, perevod.Name(n), types.MergePatchType, patch, metav1.PatchOptions{FieldManager: fieldManager})
		if err != nil {
			failed++
			o.reportErrorVars(err, golos.Vars{"Вид": "Операция", "Имя": n})
			continue
		}
		fmt.Fprintf(o.out, "%s Операция «%s» утверждена: вторая подпись — %s\n", green(o.tty, "✓"), n, me)
	}
	if failed > 0 {
		return fmt.Errorf("не утверждено операций: %d из %d", failed, len(names))
	}
	return nil
}

func (o *options) uchetStorno(ctx context.Context, name, date, content string) error {
	c, err := o.client()
	if err != nil {
		return err
	}
	m, err := o.uchetKind(c, "Операция")
	if err != nil {
		return err
	}
	orig, err := c.resource(&m, o.ns(c)).Get(ctx, perevod.Name(name), metaGet)
	if err != nil {
		return fmt.Errorf("операция «%s» не найдена", name)
	}
	origName := displayName(orig)
	if _, err := c.resource(&m, o.ns(c)).Get(ctx, perevod.Name("Сторно "+origName), metaGet); err == nil {
		return fmt.Errorf("операцию «%s» уже сторнирует «Сторно %s»: дважды не сторнируют", origName, origName)
	}
	spec, _, _ := unstructured.NestedMap(orig.Object, "spec")
	entries := bsl.NewArray()
	list, _ := spec["Проводки"].([]any)
	for _, e := range list {
		em, _ := e.(map[string]any)
		js, _ := json.Marshal(em["Сумма"])
		sum, err := bsl.ParseNumber(string(js))
		if err != nil {
			return fmt.Errorf("сумма проводки операции «%s» не читается: %s", origName, js)
		}
		p := bsl.NewMap()
		p.Insert("Дт", fmt.Sprint(em["Дт"]))
		p.Insert("Кт", fmt.Sprint(em["Кт"]))
		p.Insert("Сумма", sum.Neg())
		if s, ok := em["Содержание"].(string); ok && s != "" {
			p.Insert("Содержание", s)
		}
		entries.Elems = append(entries.Elems, p)
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	if content == "" {
		was, _ := spec["Содержание"].(string)
		content = strings.TrimSpace("Сторно: " + was)
	}
	s := bsl.NewMap()
	s.Insert("Книга", fmt.Sprint(spec["Книга"]))
	s.Insert("Дата", date)
	s.Insert("Содержание", content)
	s.Insert("Проводки", entries)
	s.Insert("Сторнирует", origName)
	md := bsl.NewMap()
	md.Insert("Имя", "Сторно "+origName)
	md.Insert("Подразделение", c.nsName(orig.GetNamespace()))
	ru := bsl.NewMap()
	ru.Insert("ВерсияAPI", perevod.GroupRu+"/v1")
	ru.Insert("Вид", "Операция")
	ru.Insert("Метаданные", md)
	ru.Insert("Спецификация", s)
	obj, err := perevod.ToAPI(ru)
	if err != nil {
		return err
	}
	return o.apply(ctx, []*bsl.Map{obj})
}

func (o *options) uchetClose(ctx context.Context, name, date string) error {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("дата «%s»: ожидается ГГГГ-ММ-ДД", date)
	}
	c, err := o.client()
	if err != nil {
		return err
	}
	ledger, err := o.ledger(ctx, c, name)
	if err != nil {
		return err
	}
	m, _ := o.uchetKind(c, "ГлавнаяКнига")
	patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"ДатаЗапрета": date}})
	if _, err := c.resource(&m, ledger.GetNamespace()).Patch(ctx, ledger.GetName(), types.MergePatchType, patch, metav1.PatchOptions{FieldManager: fieldManager}); err != nil {
		o.reportErrorVars(err, golos.Vars{"Вид": "ГлавнаяКнига", "Имя": displayName(ledger)})
		return fmt.Errorf("период не закрыт")
	}
	was, _, _ := unstructured.NestedString(ledger.Object, "spec", "ДатаЗапрета")
	if was != "" && date < was[:min(len(was), 10)] {
		fmt.Fprintf(o.out, "%s Книга «%s»: период после %s снова открыт.\n", green(o.tty, "✓"), displayName(ledger), date)
		return nil
	}
	fmt.Fprintf(o.out, "%s Книга «%s»: период по %s закрыт. Операции этой датой и раньше больше не принимаются.\n", green(o.tty, "✓"), displayName(ledger), date)
	o.say("zakrytie-perioda", nil)
	return nil
}

func (o *options) uchetSverka(ctx context.Context, name string) error {
	c, err := o.client()
	if err != nil {
		return err
	}
	ledger, err := o.ledger(ctx, c, name)
	if err != nil {
		return err
	}
	return o.printSverka(ledger)
}

func (o *options) printSverka(ledger *unstructured.Unstructured) error {
	st, _, _ := unstructured.NestedMap(ledger.Object, "status")
	str := func(k string) string { s, _ := st[k].(string); return s }
	fmt.Fprintln(o.out, bold(o.tty, "Сверка кластера с книгой «"+displayName(ledger)+"»"))
	fmt.Fprintf(o.out, "Ревизия:    %s\n", str("Ревизия"))
	posted, _ := st["Разнесено"].(bool)
	fmt.Fprintf(o.out, "Разнесено:  %s\n", map[bool]string{true: "да", false: "нет"}[posted])
	fmt.Fprintf(o.out, "Сверено:    %s\n", str("ДатаСверки"))
	if s := str("Сообщение"); s != "" {
		fmt.Fprintln(o.out, s)
	}
	diffs, _ := st["Расхождения"].([]any)
	if len(diffs) == 0 {
		if ok, _ := st["Сошлось"].(bool); ok {
			fmt.Fprintln(o.out, green(o.tty, "Расхождений нет. Сальдо кластера совпадает с учётом."))
		}
		return nil
	}
	var rows [][]string
	for _, d := range diffs {
		m, _ := d.(map[string]any)
		rows = append(rows, []string{fmt.Sprint(m["Документ"]), fmt.Sprint(m["Поле"]), fmt.Sprint(m["ПоУчёту"]), fmt.Sprint(m["ВКластере"])})
	}
	fmt.Fprintln(o.out)
	o.table([]string{"ДОКУМЕНТ", "ПОЛЕ", "ПО УЧЁТУ", "В КЛАСТЕРЕ"}, rows)
	o.say("rashozhdenie", nil)
	return nil
}

func (o *options) uchetOSV(ctx context.Context, name string) error {
	c, err := o.client()
	if err != nil {
		return err
	}
	ledger, err := o.ledger(ctx, c, name)
	if err != nil {
		return err
	}
	if _, err := o.uchetKind(c, "Операция"); err != nil {
		return err
	}
	q := `ВЫБРАТЬ О.Счёт, О.Субконто, О.Наименование,
			О.СальдоНачальноеДт, О.СальдоНачальноеКт, О.ОборотДт, О.ОборотКт, О.СальдоКонечноеДт, О.СальдоКонечноеКт
		ИЗ РегистрБухгалтерии.Хозрасчётный.ОстаткиИОбороты КАК О
		ГДЕ О.Книга = &Книга И О.Подразделение = &Подразделение
		УПОРЯДОЧИТЬ ПО О.Счёт`
	t, err := o.runQuery(ctx, q, map[string]bsl.Value{
		"Книга": displayName(ledger), "Подразделение": c.nsName(ledger.GetNamespace()),
	})
	if err != nil {
		return err
	}
	closed, _, _ := unstructured.NestedString(ledger.Object, "spec", "ДатаЗапрета")
	period := "за всё время"
	if t, err := time.Parse("2006-01-02", closed); err == nil {
		period = "открытый период с " + t.AddDate(0, 0, 1).Format("02.01.2006")
	}
	fmt.Fprintln(o.out, bold(o.tty, "Оборотно-сальдовая ведомость книги «"+displayName(ledger)+"», "+period))
	cols := []string{"СальдоНачальноеДт", "СальдоНачальноеКт", "ОборотДт", "ОборотКт", "СальдоКонечноеДт", "СальдоКонечноеКт"}
	totals := make([]bsl.Number, len(cols))
	for i := range totals {
		totals[i] = bsl.NewInt(0)
	}
	var rows [][]string
	for _, r := range t.Rows() {
		account := bsl.ToString(r.Value("Счёт"))
		sub := bsl.ToString(r.Value("Субконто")) != ""
		label := account
		if sub {
			label = "   " + bsl.ToString(r.Value("Субконто"))
		}
		row := []string{label, bsl.ToString(r.Value("Наименование"))}
		if sub {
			row[1] = ""
		}
		for i, col := range cols {
			n, _ := r.Value(col).(bsl.Number)
			row = append(row, number(n))
			if !sub {
				totals[i] = totals[i].Add(n)
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		fmt.Fprintln(o.out, "Учтённых проводок нет.")
		return nil
	}
	total := []string{"Итого", ""}
	for _, n := range totals {
		total = append(total, number(n))
	}
	rows = append(rows, total)
	o.table([]string{"СЧЁТ", "НАИМЕНОВАНИЕ", "НАЧ. ДТ", "НАЧ. КТ", "ОБОРОТ ДТ", "ОБОРОТ КТ", "КОН. ДТ", "КОН. КТ"}, rows)
	balanced := totals[0].Cmp(totals[1]) == 0 && totals[2].Cmp(totals[3]) == 0 && totals[4].Cmp(totals[5]) == 0
	if balanced {
		fmt.Fprintln(o.out, green(o.tty, "Дебет равен кредиту."))
	} else {
		fmt.Fprintln(o.out, red(o.tty, "Дебет не равен кредиту: в книге ошибка."))
	}
	fmt.Fprintln(o.out)
	return o.printSverka(ledger)
}
