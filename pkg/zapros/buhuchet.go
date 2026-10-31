package zapros

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/perevod"
)

// accountingRegister builds a virtual table of the accounting register of
// BukhOps from the ledgers and their operations.
func accountingRegister(data Data, name, table string) ([]bsl.Value, error) {
	switch strings.ToLower(name) + "." + strings.ToLower(table) {
	case "хозрасчётный.остаткииобороты", "хозрасчетный.остаткииобороты":
		return balancesAndTurnovers(data)
	}
	return nil, fmt.Errorf("Ошибка в тексте запроса: регистра бухгалтерии «%s.%s» нет; есть Хозрасчётный.ОстаткиИОбороты", name, table)
}

type ledgerInfo struct {
	name, ns, closed string
	accounts         map[string]string // code → its name
}

// balancesAndTurnovers is РегистрБухгалтерии.Хозрасчётный.ОстаткиИОбороты
// for the open period of every ledger: what was posted before the date of
// the closed period is the opening balance, what was posted since is the
// turnover. Only operations the books took count, and a сторно counts as
// what it is: the same entries with a minus, which takes them back out of
// the turnover.
func balancesAndTurnovers(data Data) ([]bsl.Value, error) {
	ledgers, err := data.Table("ГлавныеКниги")
	if err != nil {
		return nil, err
	}
	ops, err := data.Table("Операции")
	if err != nil {
		return nil, err
	}
	byKey := map[string]*ledgerInfo{}
	for _, l := range ledgers {
		name, _ := deep(l, "Метаданные", "Имя")
		ns, _ := deep(l, "Метаданные", "Подразделение")
		closed, _ := deep(l, "Спецификация", "ДатаЗапрета")
		li := &ledgerInfo{name: bsl.ToString(name), ns: bsl.ToString(ns), closed: day(closed), accounts: map[string]string{}}
		if plan, ok := deep(l, "Спецификация", "ПланСчетов"); ok {
			if it, ok := plan.(bsl.Iterable); ok {
				for _, a := range it.Items() {
					code, _ := deep(a, "Код")
					title, _ := deep(a, "Наименование")
					li.accounts[bsl.ToString(code)] = bsl.ToString(title)
				}
			}
		}
		byKey[perevod.Name(li.ns)+"/"+perevod.Name(li.name)] = li
	}

	type acc struct {
		ledger                 *ledgerInfo
		account                string
		opening, debit, credit bsl.Number
	}
	accs := map[string]*acc{}
	add := func(li *ledgerInfo, account string, amount bsl.Number, debit, opening bool) {
		for _, a := range accountLevels(account) {
			key := perevod.Name(li.ns) + "/" + perevod.Name(li.name) + "/" + a
			x := accs[key]
			if x == nil {
				x = &acc{ledger: li, account: a, opening: bsl.NewInt(0), debit: bsl.NewInt(0), credit: bsl.NewInt(0)}
				accs[key] = x
			}
			switch {
			case opening && debit:
				x.opening = x.opening.Add(amount)
			case opening:
				x.opening = x.opening.Sub(amount)
			case debit:
				x.debit = x.debit.Add(amount)
			default:
				x.credit = x.credit.Add(amount)
			}
		}
	}
	for _, op := range ops {
		if taken, _ := deep(op, "Состояние", "Учтена"); taken != true {
			continue
		}
		ns, _ := deep(op, "Метаданные", "Подразделение")
		book, _ := deep(op, "Спецификация", "Книга")
		li := byKey[perevod.Name(bsl.ToString(ns))+"/"+perevod.Name(bsl.ToString(book))]
		if li == nil {
			continue
		}
		date, _ := deep(op, "Спецификация", "Дата")
		opening := li.closed != "" && day(date) < li.closed
		entries, _ := deep(op, "Спецификация", "Проводки")
		it, ok := entries.(bsl.Iterable)
		if !ok {
			continue
		}
		for _, e := range it.Items() {
			dt, _ := deep(e, "Дт")
			kt, _ := deep(e, "Кт")
			sum, _ := deep(e, "Сумма")
			n, ok := sum.(bsl.Number)
			if !ok {
				continue
			}
			add(li, bsl.ToString(dt), n, true, opening)
			add(li, bsl.ToString(kt), n, false, opening)
		}
	}

	keys := make([]string, 0, len(accs))
	for k := range accs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]bsl.Value, 0, len(keys))
	for _, k := range keys {
		x := accs[k]
		closing := x.opening.Add(x.debit).Sub(x.credit)
		code, sub, _ := strings.Cut(x.account, "/")
		title := x.ledger.accounts[code]
		if sub != "" {
			title = sub
		}
		r := bsl.NewMap()
		r.Insert("Книга", x.ledger.name)
		r.Insert("Подразделение", x.ledger.ns)
		r.Insert("Счёт", x.account)
		r.Insert("Субконто", sub)
		r.Insert("Наименование", title)
		r.Insert("СальдоНачальноеДт", positive(x.opening))
		r.Insert("СальдоНачальноеКт", positive(x.opening.Neg()))
		r.Insert("ОборотДт", x.debit)
		r.Insert("ОборотКт", x.credit)
		r.Insert("СальдоКонечноеДт", positive(closing))
		r.Insert("СальдоКонечноеКт", positive(closing.Neg()))
		out = append(out, r)
	}
	return out, nil
}

// accountLevels: «01/Витрина» moves both the subaccount and the account 01.
func accountLevels(account string) []string {
	account = strings.TrimSpace(account)
	if code, _, ok := strings.Cut(account, "/"); ok {
		return []string{strings.TrimSpace(code), account}
	}
	return []string{account}
}

func positive(n bsl.Number) bsl.Number {
	if n.Sign() > 0 {
		return n
	}
	return bsl.NewInt(0)
}

func day(v bsl.Value) string {
	s := bsl.ToString(v)
	if len(s) > 10 {
		s = s[:10]
	}
	return s
}
