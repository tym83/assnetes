package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/perevod"
	"github.com/tym83/assnetes/pkg/zapros"
)

// clusterData gives the query language the objects of the cluster, in
// Russian form, once per kind.
type clusterData struct {
	ctx   context.Context
	c     *clients
	cache map[string][]bsl.Value
}

func (d *clusterData) Table(kind string) ([]bsl.Value, error) {
	if rows, ok := d.cache[strings.ToLower(kind)]; ok {
		return rows, nil
	}
	m, _, err := d.c.resolveKind(kind)
	if err != nil {
		return nil, err
	}
	list, err := d.c.resource(m, "").List(d.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []bsl.Value
	for i := range list.Items {
		v, err := fromUnstructured(&list.Items[i])
		if err != nil {
			return nil, err
		}
		ru, err := perevod.FromAPIIn(v, d.c.nsName)
		if err != nil {
			return nil, err
		}
		rows = append(rows, ru)
	}
	d.cache[strings.ToLower(kind)] = rows
	return rows, nil
}

func newZaprosCommand(o *options) *cobra.Command {
	var file string
	var params []string
	cmd := &cobra.Command{
		Use:     "запрос [ТЕКСТ]",
		Aliases: []string{"query"},
		Short:   "выполнить запрос на языке запросов 1С к объектам кластера",
		Example: `  1сctl запрос 'ВЫБРАТЬ П.Подразделение, КОЛИЧЕСТВО(*) КАК Подов ИЗ Кластер.Поды КАК П СГРУППИРОВАТЬ ПО П.Подразделение'
  1сctl запрос 'ВЫБРАТЬ * ИЗ РегистрНакопления.РесурсыУзлов.Остатки КАК Остатки'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			text := strings.Join(args, " ")
			if file != "" {
				data, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				text = string(data)
			}
			if strings.TrimSpace(text) == "" {
				return fmt.Errorf("текст запроса пуст: ВЫБРАТЬ ... ИЗ Кластер.<Вид>")
			}
			p := map[string]bsl.Value{}
			for _, kv := range params {
				k, v, _ := strings.Cut(kv, "=")
				if n, err := bsl.ParseNumber(v); err == nil {
					p[k] = n
				} else if strings.Contains(v, ",") {
					arr := bsl.NewArray()
					for _, s := range strings.Split(v, ",") {
						arr.Elems = append(arr.Elems, s)
					}
					p[k] = arr
				} else {
					p[k] = v
				}
			}
			return o.query(cmd.Context(), text, p)
		},
	}
	cmd.Flags().StringVar(&file, "файл", "", "файл с текстом запроса")
	cmd.Flags().StringArrayVar(&params, "параметр", nil, "Имя=Значение для &Имя; через запятую — массив")
	return cmd
}

func (o *options) query(ctx context.Context, text string, params map[string]bsl.Value) error {
	t, err := o.runQuery(ctx, text, params)
	if err != nil {
		return err
	}
	o.printTable(t)
	return nil
}

func (o *options) runQuery(ctx context.Context, text string, params map[string]bsl.Value) (*bsl.ValueTable, error) {
	c, err := o.client()
	if err != nil {
		return nil, err
	}
	q, err := zapros.Parse(text)
	if err != nil {
		return nil, err
	}
	return zapros.Execute(q, &clusterData{ctx: ctx, c: c, cache: map[string][]bsl.Value{}}, params)
}

func (o *options) printTable(t *bsl.ValueTable) {
	header := make([]string, 0, len(t.Columns()))
	for _, c := range t.Columns() {
		header = append(header, strings.ToUpper(splitWords(c)))
	}
	var rows [][]string
	for _, r := range t.Rows() {
		var row []string
		for _, c := range t.Columns() {
			row = append(row, bsl.ToString(r.Value(c)))
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		fmt.Fprintln(o.out, "Результат запроса пуст.")
		return
	}
	o.table(header, rows)
}

// newOSVCommand prints the turnover balance sheet of the resources of nodes.
func newOSVCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:     "осв",
		Aliases: []string{"оборотно-сальдовая-ведомость"},
		Short:   "оборотно-сальдовая ведомость по ресурсам узлов: дебет — что даёт узел, кредит — что запросили поды",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(o.out, bold(o.tty, "Оборотно-сальдовая ведомость по счёту 01 «Основные средства», субконто «Узлы»"))
			fmt.Fprintln(o.out, "Дебет — что даёт узел, кредит — что запросили поды. Ядра — в штуках, память — в ГиБ.")
			return o.query(cmd.Context(), `
				ВЫБРАТЬ
					О.Узел КАК Узел,
					О.ПроцессорПриход КАК ЯдраОборотДт, О.ПроцессорРасход КАК ЯдраОборотКт, О.ПроцессорОстаток КАК ЯдраСальдоДт,
					О.ПамятьПриход КАК ПамятьОборотДт, О.ПамятьРасход КАК ПамятьОборотКт, О.ПамятьОстаток КАК ПамятьСальдоДт,
					О.ПодыРасход КАК Подов
				ИЗ РегистрНакопления.РесурсыУзлов.ОстаткиИОбороты КАК О
				УПОРЯДОЧИТЬ ПО О.Узел`, nil)
		},
	}
}

// splitWords puts spaces between the words of a column name written
// together, «ЯдраОборотДт» → «Ядра Оборот Дт», as the window does.
func splitWords(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if i > 0 && unicode.IsUpper(r) && unicode.IsLower(rs[i-1]) {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
