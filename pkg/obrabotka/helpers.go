package obrabotka

import (
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

	// ВыполнитьМанифест(Текст, Параметры) returns the objects a manifest
	// posts, as the module sees objects of the cluster.
	it.Define(func(it *bsl.Interp, args []*bsl.Ref) (bsl.Value, error) {
		params := map[string]string{}
		if p, ok := bsl.Arg(args, 1).(*bsl.Structure); ok {
			for _, k := range p.Keys() {
				v, _ := p.Get(k)
				params[k] = bsl.ToString(v)
			}
		}
		res, err := manifest.Run(itContext(it), "манифест", bsl.ToString(bsl.Arg(args, 0)), manifest.Options{
			Params: params, MaxSteps: 1_000_000, Message: func(string) {},
		})
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

func comparable(v bsl.Value) (string, error) {
	return bsl.WriteJSON(plainCopy(v), "")
}
