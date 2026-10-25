package obrabotka

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/manifest"
	"github.com/tym83/assnetes/pkg/perevod"
)

// Cluster is the global object Кластер of operator modules.
type Cluster struct {
	ctx      context.Context
	c        client.Client
	mapper   meta.RESTMapper
	typed    kubernetes.Interface
	config   *rest.Config
	recorder events.EventRecorder
	manager  string // field manager: the name of the обработка
}

func (*Cluster) TypeName() string { return "Кластер" }

// ToLang turns an object of the API into what a module sees: Russian
// fields, an object that grows as it is written.
func ToLang(u *unstructured.Unstructured) (bsl.Value, error) {
	js, err := u.MarshalJSON()
	if err != nil {
		return nil, err
	}
	v, err := bsl.ParseJSON(js, true)
	if err != nil {
		return nil, err
	}
	ru, err := perevod.FromAPI(v)
	if err != nil {
		return nil, err
	}
	o := manifest.FromValue(ru).(*manifest.Object)
	if k, ok := perevod.LookupKind(u.GetKind()); ok {
		o.WithType(k.Ru)
	}
	return o, nil
}

// FromLang turns what a module built into an object of the API.
func FromLang(v bsl.Value) (*unstructured.Unstructured, error) {
	m, err := perevod.ToAPI(v)
	if err != nil {
		return nil, err
	}
	js, err := bsl.WriteJSON(m, "")
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{}
	return u, u.UnmarshalJSON([]byte(js))
}

func (cl *Cluster) gvk(kind string) (schema.GroupVersionKind, error) { return ResolveGVK(kind) }

func (cl *Cluster) namespaced(gvk schema.GroupVersionKind) bool {
	m, err := cl.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	return err == nil && m.Scope.Name() == meta.RESTScopeNameNamespace
}

func str(args []*bsl.Ref, i int) string { return bsl.ToString(bsl.Arg(args, i)) }

func (cl *Cluster) CallMethod(_ *bsl.Interp, name string, args []*bsl.Ref) (bsl.Value, error) {
	switch strings.ToLower(name) {
	case "получить", "get":
		return cl.get(str(args, 0), str(args, 1), str(args, 2))
	case "выбрать", "select", "list":
		return cl.list(str(args, 0), bsl.Arg(args, 1), str(args, 2))
	case "провести", "apply":
		return cl.apply(bsl.Arg(args, 0), bsl.Arg(args, 1))
	case "удалить", "delete":
		return nil, cl.delete(str(args, 0), str(args, 1), str(args, 2))
	case "пометитьнаудаление", "markfordeletion":
		return nil, cl.mark(bsl.Arg(args, 0))
	case "записатьсостояние", "writestatus":
		return nil, cl.writeStatus(bsl.Arg(args, 0))
	case "записатьсобытие", "recordevent":
		return nil, cl.event(bsl.Arg(args, 0), str(args, 1), str(args, 2), str(args, 3))
	case "выполнитьвподе", "execinpod":
		return cl.exec(str(args, 0), str(args, 1), bsl.Arg(args, 2), str(args, 3))
	case "прочитатьсекрет", "readsecret":
		return cl.readSecret(str(args, 0), str(args, 1), str(args, 2))
	}
	return nil, fmt.Errorf("Метод объекта не обнаружен (%s)", name)
}

func (cl *Cluster) get(kind, name, ns string) (bsl.Value, error) {
	gvk, err := cl.gvk(kind)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	key := client.ObjectKey{Name: perevod.Name(name)}
	if cl.namespaced(gvk) {
		key.Namespace = perevod.Name(ns)
	}
	if err := cl.c.Get(cl.ctx, key, u); err != nil {
		if apierrors.IsNotFound(err) {
			return bsl.Undefined{}, nil
		}
		return nil, err
	}
	return ToLang(u)
}

func (cl *Cluster) list(kind string, filter bsl.Value, ns string) (bsl.Value, error) {
	gvk, err := cl.gvk(kind)
	if err != nil {
		return nil, err
	}
	l := &unstructured.UnstructuredList{}
	l.SetGroupVersionKind(gvk.GroupVersion().WithKind(gvk.Kind + "List"))
	var opts []client.ListOption
	if ns != "" && cl.namespaced(gvk) {
		opts = append(opts, client.InNamespace(perevod.Name(ns)))
	}
	sel := labels.Set{}
	switch f := filter.(type) {
	case string:
		if f != "" {
			s, err := labels.Parse(f)
			if err != nil {
				return nil, err
			}
			opts = append(opts, client.MatchingLabelsSelector{Selector: s})
		}
	case *bsl.Structure:
		for _, k := range f.Keys() {
			v, _ := f.Get(k)
			sel[perevod.AnnotationKey(k)] = perevod.LabelValue(bsl.ToString(v))
		}
	case *bsl.Map:
		for _, k := range f.Keys() {
			v, _ := f.Lookup(k)
			sel[perevod.AnnotationKey(bsl.ToString(k))] = perevod.LabelValue(bsl.ToString(v))
		}
	}
	if len(sel) > 0 {
		opts = append(opts, client.MatchingLabels(sel))
	}
	if err := cl.c.List(cl.ctx, l, opts...); err != nil {
		return nil, err
	}
	out := bsl.NewArray()
	for i := range l.Items {
		v, err := ToLang(&l.Items[i])
		if err != nil {
			return nil, err
		}
		out.Elems = append(out.Elems, v)
	}
	return out, nil
}

// apply posts an object with server-side apply. With an owner, the object
// belongs to it and goes when the owner goes.
func (cl *Cluster) apply(obj, owner bsl.Value) (bsl.Value, error) {
	u, err := FromLang(obj)
	if err != nil {
		return nil, err
	}
	gvk := u.GroupVersionKind()
	if owner != nil {
		if _, undef := owner.(bsl.Undefined); !undef {
			o, err := FromLang(owner)
			if err != nil {
				return nil, fmt.Errorf("владелец: %w", err)
			}
			if u.GetNamespace() == "" && cl.namespaced(gvk) {
				u.SetNamespace(o.GetNamespace())
			}
			if o.GetUID() == "" {
				live := &unstructured.Unstructured{}
				live.SetGroupVersionKind(o.GroupVersionKind())
				if err := cl.c.Get(cl.ctx, client.ObjectKeyFromObject(o), live); err != nil {
					return nil, fmt.Errorf("владелец: %w", err)
				}
				o = live
			}
			yes := true
			u.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: o.GetAPIVersion(), Kind: o.GetKind(), Name: o.GetName(), UID: o.GetUID(),
				Controller: &yes, BlockOwnerDeletion: &yes,
			}})
		}
	}
	u.SetManagedFields(nil)
	u.SetResourceVersion("")
	if err := cl.c.Apply(cl.ctx, client.ApplyConfigurationFromUnstructured(u), client.FieldOwner(cl.manager), client.ForceOwnership); err != nil {
		return nil, err
	}
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(gvk)
	if err := cl.c.Get(cl.ctx, client.ObjectKeyFromObject(u), live); err != nil {
		return nil, err
	}
	return ToLang(live)
}

func (cl *Cluster) delete(kind, name, ns string) error {
	gvk, err := cl.gvk(kind)
	if err != nil {
		return err
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName(perevod.Name(name))
	if cl.namespaced(gvk) {
		u.SetNamespace(perevod.Name(ns))
	}
	err = cl.c.Delete(cl.ctx, u, client.PropagationPolicy(metav1.DeletePropagationBackground))
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// AnnMarked is the mark «пометка на удаление».
const AnnMarked = perevod.Group + "/pometka-udaleniya"

func (cl *Cluster) mark(obj bsl.Value) error {
	u, err := FromLang(obj)
	if err != nil {
		return err
	}
	patch := []byte(`{"metadata":{"annotations":{"` + AnnMarked + `":"true"}}}`)
	return cl.c.Patch(cl.ctx, u, client.RawPatch("application/merge-patch+json", patch))
}

// writeStatus writes the Состояние of an object.
func (cl *Cluster) writeStatus(obj bsl.Value) error {
	u, err := FromLang(obj)
	if err != nil {
		return err
	}
	return cl.c.Status().Update(cl.ctx, u)
}

func (cl *Cluster) event(obj bsl.Value, typ, reason, message string) error {
	if cl.recorder == nil {
		return nil
	}
	u, err := FromLang(obj)
	if err != nil {
		return err
	}
	t := corev1.EventTypeNormal
	if strings.HasPrefix(strings.ToLower(typ), "пред") || strings.EqualFold(typ, "warning") {
		t = corev1.EventTypeWarning
	}
	cl.recorder.Eventf(u, nil, t, reason, "Обработка", "%s", message)
	return nil
}

// exec runs a command in a container of a pod and returns
// Структура(КодВозврата, Вывод, Ошибки).
func (cl *Cluster) exec(pod, ns string, command bsl.Value, container string) (bsl.Value, error) {
	var cmd []string
	switch c := command.(type) {
	case string:
		cmd = []string{"sh", "-c", c}
	case bsl.Iterable:
		for _, e := range c.Items() {
			cmd = append(cmd, bsl.ToString(e))
		}
	default:
		return nil, fmt.Errorf("команда должна быть строкой или массивом")
	}
	req := cl.typed.CoreV1().RESTClient().Post().Resource("pods").
		Name(perevod.Name(pod)).Namespace(perevod.Name(ns)).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container, Command: cmd, Stdout: true, Stderr: true,
		}, scheme.ParameterCodec)
	ex, err := remotecommand.NewWebSocketExecutor(cl.config, "GET", req.URL().String())
	if err != nil {
		return nil, err
	}
	var stdout, stderr bytes.Buffer
	code := 0
	err = ex.StreamWithContext(cl.ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		var ce interface{ ExitStatus() int }
		if asExit(err, &ce) {
			code = ce.ExitStatus()
		} else {
			return nil, err
		}
	}
	res := bsl.NewStructure()
	_ = res.Insert("КодВозврата", bsl.NewInt(int64(code)))
	_ = res.Insert("Вывод", stdout.String())
	_ = res.Insert("Ошибки", stderr.String())
	return res, nil
}

func asExit(err error, target *interface{ ExitStatus() int }) bool {
	if e, ok := err.(interface{ ExitStatus() int }); ok {
		*target = e
		return true
	}
	return false
}

func (cl *Cluster) readSecret(name, key, ns string) (bsl.Value, error) {
	s, err := cl.typed.CoreV1().Secrets(perevod.Name(ns)).Get(cl.ctx, perevod.Name(name), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return bsl.Undefined{}, nil
		}
		return nil, err
	}
	if v, ok := s.Data[key]; ok {
		return string(v), nil
	}
	if v, ok := s.StringData[key]; ok {
		return v, nil
	}
	return bsl.Undefined{}, nil
}

// statusJSON is the Состояние of an object as JSON, to see if it changed.
func statusJSON(v bsl.Value) string {
	o, ok := v.(*manifest.Object)
	if !ok {
		return ""
	}
	st, ok := o.Get("Состояние")
	if !ok {
		return ""
	}
	js, _ := bsl.WriteJSON(plainCopy(st), "")
	return js
}

func plainCopy(v bsl.Value) bsl.Value {
	if e, ok := v.(interface {
		Entries() ([]string, []bsl.Value)
	}); ok {
		m := bsl.NewMap()
		keys, vals := e.Entries()
		for i, k := range keys {
			m.Insert(k, plainCopy(vals[i]))
		}
		return m
	}
	if l, ok := v.(*manifest.List); ok {
		a := bsl.NewArray()
		for _, e := range l.Items() {
			a.Elems = append(a.Elems, plainCopy(e))
		}
		return a
	}
	return v
}
