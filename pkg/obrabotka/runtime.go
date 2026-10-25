package obrabotka

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/perevod"
)

// Runner hosts one обработка: its interpreter, shared by the reconciler and
// the schedule, one call at a time, as the module variables expect.
type Runner struct {
	ob      *Obrabotka
	mu      sync.Mutex
	it      *bsl.Interp
	script  *bsl.Script
	cluster *Cluster
	log     logr.Logger

	// state of the call in progress
	requeue  time.Duration
	messages []string
}

// MaxSteps bounds the statements of one call of a handler.
var MaxSteps int64 = 5_000_000

// RunScheduleAtStart runs every scheduled procedure once when the leader
// starts, besides its schedule: for testing and for catching up.
var RunScheduleAtStart bool

// Finalizer of an обработка that has ПередУдалением.
func (r *Runner) finalizer() string {
	return perevod.Group + "/" + perevod.Translit(r.ob.Name)
}

// NewRunner loads the module of an обработка into a context with Кластер,
// Журнал and the rest of the runtime.
func NewRunner(ob *Obrabotka, mgr manager.Manager) (*Runner, error) {
	typed, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}
	r := &Runner{ob: ob, log: mgr.GetLogger().WithValues("обработка", ob.Name)}
	r.cluster = &Cluster{
		c: mgr.GetClient(), mapper: mgr.GetRESTMapper(), typed: typed, config: mgr.GetConfig(),
		recorder: mgr.GetEventRecorder(ob.Name), manager: "obrabotka-" + perevod.Translit(ob.Name),
	}
	it := bsl.New()
	it.MaxSteps = MaxSteps
	it.Message = func(s string) {
		r.messages = append(r.messages, s)
		r.log.Info(s)
	}
	it.SetGlobal(r.cluster, "Кластер", "Cluster")
	it.SetGlobal(&journal{log: r.log}, "Журнал", "Log")
	it.Define(func(_ *bsl.Interp, args []*bsl.Ref) (bsl.Value, error) {
		n, ok := bsl.Arg(args, 0).(bsl.Number)
		if !ok {
			return nil, fmt.Errorf("ПовторитьЧерез: ожидается число секунд")
		}
		r.requeue = time.Duration(n.Int64()) * time.Second
		return nil, nil
	}, "ПовторитьЧерез", "RequeueAfter")
	registerHTTP(it)
	registerXML(it)
	registerHelpers(it)
	r.it = it
	r.cluster.ctx = context.Background()
	s, err := it.Load(ob.Module)
	if err != nil {
		return nil, fmt.Errorf("%s: модуль не загрузился: %w", ob.Name, err)
	}
	r.script = s
	return r, nil
}

// call runs a method of the module with the runtime bound to ctx.
func (r *Runner) call(ctx context.Context, method string, args ...*bsl.Ref) error {
	r.cluster.ctx = ctx
	r.it.WithContext(ctx)
	r.requeue = 0
	r.messages = nil
	_, err := r.script.CallRefs(method, args)
	return err
}

// Reconcile is controller-runtime's entry into ОбработкаПроведения.
func (r *Runner) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(r.ob.Kind)
	if err := r.cluster.c.Get(ctx, req.NamespacedName, u); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	hasDelete := r.ob.Module.Method(HandlerDelete) != nil

	if !u.GetDeletionTimestamp().IsZero() {
		if !hasDelete || !controllerutil.ContainsFinalizer(u, r.finalizer()) {
			return reconcile.Result{}, nil
		}
		obj, err := ToLang(u)
		if err != nil {
			return reconcile.Result{}, err
		}
		cancel := &bsl.Ref{V: false}
		if err := r.call(ctx, HandlerDelete, &bsl.Ref{V: obj}, cancel); err != nil {
			r.warn(u, "ОшибкаУдаления", err.Error())
			return reconcile.Result{}, err
		}
		if cancel.V == true {
			r.warn(u, "ОтказУдаления", r.said("Удаление отложено"))
			return reconcile.Result{RequeueAfter: r.after(30 * time.Second)}, nil
		}
		controllerutil.RemoveFinalizer(u, r.finalizer())
		return reconcile.Result{}, r.cluster.c.Update(ctx, u)
	}

	if hasDelete && !controllerutil.ContainsFinalizer(u, r.finalizer()) {
		controllerutil.AddFinalizer(u, r.finalizer())
		if err := r.cluster.c.Update(ctx, u); err != nil {
			return reconcile.Result{}, err
		}
	}

	obj, err := ToLang(u)
	if err != nil {
		return reconcile.Result{}, err
	}
	before := statusJSON(obj)
	cancel := &bsl.Ref{V: false}
	if err := r.call(ctx, HandlerPost, &bsl.Ref{V: obj}, cancel); err != nil {
		r.warn(u, "ОшибкаПроведения", err.Error())
		return reconcile.Result{}, err
	}
	if after := statusJSON(obj); after != before {
		if err := r.writeStatus(ctx, obj); err != nil {
			return reconcile.Result{}, err
		}
	}
	if cancel.V == true {
		r.warn(u, "Отказ", r.said("Документ не проведён"))
		return reconcile.Result{RequeueAfter: r.after(time.Minute)}, nil
	}
	return reconcile.Result{RequeueAfter: r.requeue}, nil
}

func (r *Runner) after(def time.Duration) time.Duration {
	if r.requeue > 0 {
		return r.requeue
	}
	return def
}

func (r *Runner) said(def string) string {
	if len(r.messages) == 0 {
		return def
	}
	return strings.Join(r.messages, "; ")
}

func (r *Runner) warn(u *unstructured.Unstructured, reason, msg string) {
	r.log.Info("отказ", "причина", reason, "сообщение", msg, "объект", u.GetName())
	if r.cluster.recorder != nil {
		r.cluster.recorder.Eventf(u, nil, "Warning", reason, "Обработка", "%s", msg)
	}
}

func (r *Runner) writeStatus(ctx context.Context, obj bsl.Value) error {
	u, err := FromLang(obj)
	if err != nil {
		return err
	}
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(u.GroupVersionKind())
	if err := r.cluster.c.Get(ctx, client.ObjectKeyFromObject(u), live); err != nil {
		return err
	}
	if st, ok := u.Object["status"]; ok {
		live.Object["status"] = st
	} else {
		delete(live.Object, "status")
	}
	err = r.cluster.c.Status().Update(ctx, live)
	if apierrors.IsNotFound(err) {
		// a kind without the status subresource keeps its status in the object
		return r.cluster.c.Update(ctx, live)
	}
	return err
}

// Setup registers the обработка with a manager: its controller and its
// schedule.
func Setup(mgr manager.Manager, ob *Obrabotka) (*Runner, error) {
	r, err := NewRunner(ob, mgr)
	if err != nil {
		return nil, err
	}
	if !ob.Kind.Empty() {
		forObj := &unstructured.Unstructured{}
		forObj.SetGroupVersionKind(ob.Kind)
		b := ctrl.NewControllerManagedBy(mgr).
			Named(perevod.Translit(ob.Name)).
			For(forObj).
			WithOptions(controller.Options{MaxConcurrentReconciles: 1})
		for _, gvk := range ob.Owns {
			o := &unstructured.Unstructured{}
			o.SetGroupVersionKind(gvk)
			b = b.Owns(o)
		}
		for _, gvk := range ob.Watches {
			o := &unstructured.Unstructured{}
			o.SetGroupVersionKind(gvk)
			b = b.Watches(o, handler.EnqueueRequestsFromMapFunc(r.all), builder.WithPredicates())
		}
		if err := b.Complete(r); err != nil {
			return nil, err
		}
	}
	if len(ob.Schedule) > 0 {
		if err := mgr.Add(&schedule{r: r}); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// all enqueues every object of the kind: a watched kind changed, and the
// module decides what that means.
func (r *Runner) all(ctx context.Context, _ client.Object) []reconcile.Request {
	l := &unstructured.UnstructuredList{}
	l.SetGroupVersionKind(r.ob.Kind.GroupVersion().WithKind(r.ob.Kind.Kind + "List"))
	if err := r.cluster.c.List(ctx, l); err != nil {
		return nil
	}
	out := make([]reconcile.Request, 0, len(l.Items))
	for _, o := range l.Items {
		out = append(out, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&o)})
	}
	return out
}

// schedule runs the procedures marked &Расписание, on the leader only.
type schedule struct{ r *Runner }

func (s *schedule) NeedLeaderElection() bool { return true }

func (s *schedule) Start(ctx context.Context) error {
	c := cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)))
	for _, job := range s.r.ob.Schedule {
		method := job.Method
		if _, err := c.AddFunc(job.Cron, func() {
			s.r.mu.Lock()
			defer s.r.mu.Unlock()
			if err := s.r.call(ctx, method); err != nil {
				s.r.log.Error(err, "регламентное задание завершилось с ошибкой", "процедура", method)
			}
		}); err != nil {
			return fmt.Errorf("%s: расписание «%s»: %w", method, job.Cron, err)
		}
	}
	c.Start()
	if RunScheduleAtStart {
		for _, job := range s.r.ob.Schedule {
			s.r.mu.Lock()
			if err := s.r.call(ctx, job.Method); err != nil {
				s.r.log.Error(err, "регламентное задание завершилось с ошибкой", "процедура", job.Method)
			}
			s.r.mu.Unlock()
		}
	}
	<-ctx.Done()
	<-c.Stop().Done()
	return nil
}

// journal is Журнал: the log of the operator.
type journal struct{ log logr.Logger }

func (*journal) TypeName() string { return "Журнал" }

func (j *journal) CallMethod(_ *bsl.Interp, name string, args []*bsl.Ref) (bsl.Value, error) {
	text := bsl.ToString(bsl.Arg(args, 0))
	switch strings.ToLower(name) {
	case "информация", "info":
		j.log.Info(text)
	case "предупреждение", "warning":
		j.log.Info(text, "важность", "Предупреждение")
	case "ошибка", "error":
		j.log.Error(nil, text)
	case "отладка", "debug":
		j.log.V(1).Info(text)
	default:
		return nil, fmt.Errorf("Метод объекта не обнаружен (%s)", name)
	}
	return nil, nil
}
