package cli

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/perevod"
)

type clients struct {
	config    *rest.Config
	dynamic   dynamic.Interface
	typed     kubernetes.Interface
	mapper    meta.ResettableRESTMapper
	namespace string // of the context, when no flag says otherwise
}

func (o *options) client() (*clients, error) {
	if o.clients != nil {
		return o.clients, nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if o.kubeconfig != "" {
		rules.ExplicitPath = o.kubeconfig
	}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{CurrentContext: o.context})
	cfg, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("не найден список информационных баз (kubeconfig): %w", err)
	}
	cfg.UserAgent = "1сctl/" + Version
	ns, _, _ := cc.Namespace()
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	o.clients = &clients{
		config:    cfg,
		dynamic:   dyn,
		typed:     typed,
		mapper:    restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disc)),
		namespace: ns,
	}
	return o.clients, nil
}

// ns is the namespace commands work in, in the form of the API.
func (o *options) ns(c *clients) string {
	if o.namespace != "" {
		return perevod.Name(o.namespace)
	}
	if c.namespace != "" {
		return c.namespace
	}
	return "default"
}

// resolveKind finds the resource for a kind named in any language.
func (c *clients) resolveKind(name string) (*meta.RESTMapping, *perevod.Kind, error) {
	k, known := perevod.LookupKind(name)
	var gvk schema.GroupVersionKind
	if known {
		gv, err := schema.ParseGroupVersion(k.APIVersion)
		if err != nil {
			return nil, nil, err
		}
		gvk = gv.WithKind(k.Kind)
		m, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return nil, nil, fmt.Errorf("вид «%s» в этом кластере не ведётся: %w", k.Ru, err)
		}
		return m, k, nil
	}
	gvr, err := c.mapper.ResourceFor(schema.GroupVersionResource{Resource: strings.ToLower(name)})
	if err != nil {
		return nil, nil, fmt.Errorf("вид «%s» нам не знаком", name)
	}
	kinds, err := c.mapper.KindFor(gvr)
	if err != nil {
		return nil, nil, err
	}
	m, err := c.mapper.RESTMapping(kinds.GroupKind(), kinds.Version)
	return m, nil, err
}

// mappingOf finds the resource of an object by its full apiVersion and kind:
// a kind name alone may be served by several groups.
func (c *clients) mappingOf(u *unstructured.Unstructured) (*meta.RESTMapping, *perevod.Kind, error) {
	gvk := u.GroupVersionKind()
	m, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		c.mapper.Reset()
		if m, err = c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version); err != nil {
			return nil, nil, fmt.Errorf("вид «%s» (%s) в этом кластере не ведётся", gvk.Kind, gvk.GroupVersion())
		}
	}
	var kind *perevod.Kind
	if k, ok := perevod.LookupKind(gvk.Kind); ok && k.APIVersion == gvk.GroupVersion().String() {
		kind = k
	}
	return m, kind, nil
}

func (c *clients) resource(m *meta.RESTMapping, ns string) dynamic.ResourceInterface {
	if m.Scope.Name() == meta.RESTScopeNameNamespace {
		return c.dynamic.Resource(m.Resource).Namespace(ns)
	}
	return c.dynamic.Resource(m.Resource)
}

// toUnstructured turns an API-form object of the language into what the
// client sends.
func toUnstructured(obj *bsl.Map) (*unstructured.Unstructured, error) {
	js, err := bsl.WriteJSON(obj, "")
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{}
	if err := u.UnmarshalJSON([]byte(js)); err != nil {
		return nil, err
	}
	return u, nil
}

// fromUnstructured turns what the client got into a value of the language.
func fromUnstructured(u *unstructured.Unstructured) (bsl.Value, error) {
	js, err := u.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return bsl.ParseJSON(js, true)
}

func (c *clients) get(ctx context.Context, m *meta.RESTMapping, ns, name string) (*unstructured.Unstructured, error) {
	return c.resource(m, ns).Get(ctx, name, metaGet)
}
