package obrabotka

import (
	"context"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/tym83/assnetes/pkg/metadannye"
)

// InstallKinds posts the kinds an обработка describes in its
// Метаданные*.yaml and waits until the API server serves them.
func InstallKinds(ctx context.Context, cfg *rest.Config, ob *Obrabotka) error {
	if len(ob.CRDs) == 0 {
		return nil
	}
	c, err := client.New(cfg, client.Options{})
	if err != nil {
		return err
	}
	for _, f := range ob.CRDs {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		d, err := metadannye.Parse(data)
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		js, err := d.CRD()
		if err != nil {
			return err
		}
		u := &unstructured.Unstructured{}
		if err := u.UnmarshalJSON(js); err != nil {
			return err
		}
		if err := c.Apply(ctx, client.ApplyConfigurationFromUnstructured(u), client.FieldOwner("obrabotchik"), client.ForceOwnership); err != nil {
			return fmt.Errorf("вид «%s» не установлен: %w", d.Ru, err)
		}
		err = wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
			crd := &unstructured.Unstructured{}
			crd.SetGroupVersionKind(u.GroupVersionKind())
			if err := c.Get(ctx, client.ObjectKey{Name: u.GetName()}, crd); err != nil {
				return false, nil
			}
			conds, _, _ := unstructured.NestedSlice(crd.Object, "status", "conditions")
			for _, cond := range conds {
				m, _ := cond.(map[string]any)
				if m["type"] == "Established" && m["status"] == "True" {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return fmt.Errorf("вид «%s» не заработал: %w", d.Ru, err)
		}
	}
	return nil
}
