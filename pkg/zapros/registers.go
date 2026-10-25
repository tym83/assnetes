package zapros

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/tym83/assnetes/pkg/bsl"
)

// quantity turns «500m» into 0.5 and «2Gi» into bytes.
func quantity(v bsl.Value) bsl.Value {
	s := bsl.ToString(v)
	if s == "" {
		return bsl.NewInt(0)
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return bsl.Undefined{}
	}
	n, err := bsl.ParseNumber(q.AsDec().String())
	if err != nil {
		return bsl.Undefined{}
	}
	return n
}

var gib = bsl.NewInt(1 << 30)

func num(v bsl.Value) bsl.Number {
	n, _ := quantity(v).(bsl.Number)
	return n
}

// register builds the virtual table of an accumulation register from the
// objects of the cluster.
func register(data Data, name, table string) ([]bsl.Value, error) {
	switch strings.ToLower(name) + "." + strings.ToLower(table) {
	case "ресурсыузлов.остатки", "ресурсыузлов.остаткииобороты":
		return nodeResources(data)
	case "перезапуски.обороты":
		return restarts(data)
	}
	return nil, fmt.Errorf("Ошибка в тексте запроса: регистра «%s.%s» нет; есть РесурсыУзлов.Остатки и Перезапуски.Обороты", name, table)
}

// nodeResources is РегистрНакопления.РесурсыУзлов.Остатки: what a node can
// give (приход), what pods asked of it (расход), and the balance.
func nodeResources(data Data) ([]bsl.Value, error) {
	nodes, err := data.Table("Узлы")
	if err != nil {
		return nil, err
	}
	pods, err := data.Table("Поды")
	if err != nil {
		return nil, err
	}
	type acc struct{ cpu, mem bsl.Number }
	used := map[string]*acc{}
	count := map[string]int{}
	for _, p := range pods {
		phase, _ := deep(p, "Состояние", "Фаза")
		if s := bsl.ToString(phase); s == "Succeeded" || s == "Failed" {
			continue
		}
		node, _ := deep(p, "Спецификация", "ИмяУзла")
		n := bsl.ToString(node)
		if n == "" {
			continue
		}
		a := used[n]
		if a == nil {
			a = &acc{cpu: bsl.NewInt(0), mem: bsl.NewInt(0)}
			used[n] = a
		}
		count[n]++
		cs, _ := deep(p, "Спецификация", "Контейнеры")
		if it, ok := cs.(bsl.Iterable); ok {
			for _, c := range it.Items() {
				cpu, _ := deep(c, "Ресурсы", "Запросы", "Процессор")
				mem, _ := deep(c, "Ресурсы", "Запросы", "Память")
				a.cpu = a.cpu.Add(num(cpu))
				a.mem = a.mem.Add(num(mem))
			}
		}
	}
	var out []bsl.Value
	for _, n := range nodes {
		nameV, _ := deep(n, "Метаданные", "Имя")
		name := bsl.ToString(nameV)
		cpuV, _ := deep(n, "Состояние", "Доступно", "Процессор")
		memV, _ := deep(n, "Состояние", "Доступно", "Память")
		podsV, _ := deep(n, "Состояние", "Доступно", "pods")
		cpu, mem := num(cpuV), num(memV)
		a := used[name]
		if a == nil {
			a = &acc{cpu: bsl.NewInt(0), mem: bsl.NewInt(0)}
		}
		memIn, _ := mem.Div(gib)
		memOut, _ := a.mem.Div(gib)
		r := bsl.NewMap()
		r.Insert("Узел", name)
		r.Insert("ПроцессорПриход", cpu.Round(3))
		r.Insert("ПроцессорРасход", a.cpu.Round(3))
		r.Insert("ПроцессорОстаток", cpu.Sub(a.cpu).Round(3))
		r.Insert("ПамятьПриход", memIn.Round(2))
		r.Insert("ПамятьРасход", memOut.Round(2))
		r.Insert("ПамятьОстаток", memIn.Sub(memOut).Round(2))
		r.Insert("ПодыПриход", num(podsV))
		r.Insert("ПодыРасход", bsl.NewInt(int64(count[name])))
		r.Insert("ПодыОстаток", num(podsV).Sub(bsl.NewInt(int64(count[name]))))
		out = append(out, r)
	}
	return out, nil
}

// restarts is РегистрНакопления.Перезапуски.Обороты: restarts of every
// container, by namespace and pod.
func restarts(data Data) ([]bsl.Value, error) {
	pods, err := data.Table("Поды")
	if err != nil {
		return nil, err
	}
	var out []bsl.Value
	for _, p := range pods {
		ns, _ := deep(p, "Метаданные", "Подразделение")
		name, _ := deep(p, "Метаданные", "Имя")
		st, _ := deep(p, "Состояние", "СостоянияКонтейнеров")
		it, ok := st.(bsl.Iterable)
		if !ok {
			continue
		}
		for _, c := range it.Items() {
			cn, _ := deep(c, "Имя")
			n, _ := deep(c, "ЧислоПерезапусков")
			r := bsl.NewMap()
			r.Insert("Подразделение", ns)
			r.Insert("Под", name)
			r.Insert("Контейнер", cn)
			nn, ok := n.(bsl.Number)
			if !ok {
				nn = bsl.NewInt(0)
			}
			r.Insert("ПерезапускиОборот", nn)
			out = append(out, r)
		}
	}
	return out, nil
}
