package obrabotka

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/manifest"
	"github.com/tym83/assnetes/pkg/perevod"
)

// registerHelpers adds what modules of operators need beyond the language:
// kinds, comparison of data, quantities of resources, names of the API and
// manifests.
func registerHelpers(it *bsl.Interp) {
	manifest.DefineKinds(it)

	// ДанныеСовпадают, as in the standard subsystems library: deep equality
	// of structures, maps, arrays and objects of the cluster.
	it.Define(func(_ *bsl.Interp, args []*bsl.Ref) (bsl.Value, error) {
		a, err := comparable(bsl.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		b, err := comparable(bsl.Arg(args, 1))
		if err != nil {
			return nil, err
		}
		return a == b, nil
	}, "ДанныеСовпадают", "DataMatch")

	// Отпечаток: what a document was when the books took it, to see later
	// that nobody rewrote it.
	it.Define(func(_ *bsl.Interp, args []*bsl.Ref) (bsl.Value, error) {
		return fingerprint(bsl.Arg(args, 0))
	}, "Отпечаток", "Fingerprint")

	// РазобратьКоличество("100m") = 0.1, ("1Gi") = 1073741824.
	it.Define(func(_ *bsl.Interp, args []*bsl.Ref) (bsl.Value, error) {
		s := bsl.ToString(bsl.Arg(args, 0))
		if s == "" {
			return bsl.NewInt(0), nil
		}
		q, err := resource.ParseQuantity(s)
		if err != nil {
			return nil, fmt.Errorf("Неверное количество ресурса «%s»", s)
		}
		return bsl.ParseNumber(q.AsDec().String())
	}, "РазобратьКоличество", "ParseQuantity")

	// ИмяAPI: the name a name as written has in the API.
	it.Define(func(_ *bsl.Interp, args []*bsl.Ref) (bsl.Value, error) {
		return perevod.Name(bsl.ToString(bsl.Arg(args, 0))), nil
	}, "ИмяAPI", "APIName")

	// ВыполнитьМанифест(Текст, Параметры, Сальдо, Подразделение) returns the
	// objects a manifest posts, as the module sees objects of the cluster.
	// Сальдо, a Соответствие of accounts and balances, gives the manifest
	// Сальдо("01/Витрина"); Подразделение fills objects that name none.
	it.Define(func(it *bsl.Interp, args []*bsl.Ref) (bsl.Value, error) {
		params := map[string]string{}
		if p, ok := bsl.Arg(args, 1).(*bsl.Structure); ok {
			for _, k := range p.Keys() {
				v, _ := p.Get(k)
				params[k] = bsl.ToString(v)
			}
		}
		opts := manifest.Options{
			Params: params, MaxSteps: 1_000_000, Message: func(string) {},
			Namespace: perevod.Name(bsl.ToString(bsl.Arg(args, 3))),
		}
		if balances, ok := bsl.Arg(args, 2).(*bsl.Map); ok {
			opts.Define = func(m *bsl.Interp) { defineBalance(m, balances) }
		}
		res, err := manifest.Run(itContext(it), "манифест", bsl.ToString(bsl.Arg(args, 0)), opts)
		if err != nil {
			return nil, err
		}
		out := bsl.NewArray()
		for _, o := range res.Objects {
			ru, err := perevod.FromAPI(o)
			if err != nil {
				return nil, err
			}
			out.Elems = append(out.Elems, manifest.FromValue(ru))
		}
		return out, nil
	}, "ВыполнитьМанифест", "RunManifest")
}

// defineBalance gives a manifest Сальдо(Счёт): the balance of an account
// in the books, zero for an account nothing was posted to.
func defineBalance(it *bsl.Interp, balances *bsl.Map) {
	it.Define(func(_ *bsl.Interp, args []*bsl.Ref) (bsl.Value, error) {
		account := bsl.ToString(bsl.Arg(args, 0))
		if account == "" {
			return nil, fmt.Errorf("Сальдо: не указан счёт")
		}
		for _, k := range balances.Keys() {
			if bsl.ToString(k) == account {
				v, _ := balances.Lookup(k)
				return v, nil
			}
		}
		return bsl.NewInt(0), nil
	}, "Сальдо", "Balance")
}

// fingerprint is Отпечаток: the SHA-256 of a value written as JSON.
func fingerprint(v bsl.Value) (string, error) {
	js, err := comparable(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(js))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func comparable(v bsl.Value) (string, error) {
	return bsl.WriteJSON(plainCopy(v), "")
}
