package obrabotka

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/perevod"
)

// WebhookOptions say where the API server reaches the обработчик.
type WebhookOptions struct {
	Addr      string // address to listen on, ":9443"
	Service   string // name of the Service in front of the обработчик
	Namespace string // its namespace
}

// operations of admission, by their Russian names in &ПроверкаЗаполнения.
var operations = map[string]admissionregistrationv1.OperationType{
	"создание": admissionregistrationv1.Create, "изменение": admissionregistrationv1.Update,
	"удаление": admissionregistrationv1.Delete, "create": admissionregistrationv1.Create,
	"update": admissionregistrationv1.Update, "delete": admissionregistrationv1.Delete,
}

var operationRu = map[admissionv1.Operation]string{
	admissionv1.Create: "Создание", admissionv1.Update: "Изменение", admissionv1.Delete: "Удаление",
	admissionv1.Connect: "Подключение",
}

// webhookRule is what an обработка checks: from &ПроверкаЗаполнения on its
// ОбработкаПроверкиЗаполнения.
type webhookRule struct {
	kinds []schema.GroupVersionKind
	ops   []admissionregistrationv1.OperationType
	// namespaces whose objects are never checked, so the cluster can
	// always be repaired
	exempt []string
}

func (ob *Obrabotka) webhook(method string) (*webhookRule, error) {
	m := ob.Module.Method(method)
	if m == nil {
		return nil, nil
	}
	a := m.Annotation("ПроверкаЗаполнения")
	rule := &webhookRule{exempt: []string{"kube-system"}}
	kinds := ""
	if a != nil {
		if v, ok := a.Param("Виды", 0); ok {
			kinds = bsl.ToString(v)
		}
		if v, ok := a.Param("Операции", 1); ok {
			for _, op := range strings.Split(bsl.ToString(v), ",") {
				o, ok := operations[strings.ToLower(strings.TrimSpace(op))]
				if !ok {
					return nil, fmt.Errorf("%s: неизвестная операция «%s»", ob.Name, op)
				}
				rule.ops = append(rule.ops, o)
			}
		}
		if v, ok := a.Param("Кроме", -1); ok {
			for _, n := range strings.Split(bsl.ToString(v), ",") {
				if n = strings.TrimSpace(n); n != "" {
					rule.exempt = append(rule.exempt, perevod.Name(n))
				}
			}
		}
	}
	if kinds == "" && len(ob.Handlers) == 1 {
		rule.kinds = []schema.GroupVersionKind{ob.Handlers[0].Kind}
	}
	for _, k := range strings.Split(kinds, ",") {
		if k = strings.TrimSpace(k); k == "" {
			continue
		}
		g, err := ResolveGVK(k)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ob.Name, err)
		}
		rule.kinds = append(rule.kinds, g)
	}
	if len(rule.kinds) == 0 {
		return nil, fmt.Errorf("%s: в &ПроверкаЗаполнения не указаны Виды", ob.Name)
	}
	if len(rule.ops) == 0 {
		rule.ops = []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update}
	}
	return rule, nil
}

// check is one method of a module that checks objects.
type check struct {
	r      *Runner
	method string
	rule   *webhookRule
}

func (c *check) path() string {
	return "/proverka/" + perevod.Translit(c.r.ob.Name) + "/" + perevod.Translit(c.method)
}

func (c *check) name() string {
	return perevod.Translit(c.r.ob.Name) + "-" + perevod.Translit(c.method) + ".obrabotchik." + perevod.Group
}

// admit answers one AdmissionReview by the check.
func (c *check) admit(w http.ResponseWriter, req *http.Request) {
	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 8<<20)).Decode(&review); err != nil || review.Request == nil {
		http.Error(w, "неверный AdmissionReview", http.StatusBadRequest)
		return
	}
	resp := &admissionv1.AdmissionResponse{UID: review.Request.UID, Allowed: true}
	if msg, deny := c.r.validate(req.Context(), c.method, review.Request); deny {
		resp.Allowed = false
		resp.Result = &metav1.Status{Message: msg, Reason: "ОтказВПроведении", Code: http.StatusForbidden}
	}
	review.Response = resp
	review.Request = nil
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(&review)
}

func (r *Runner) validate(ctx context.Context, method string, ar *admissionv1.AdmissionRequest) (string, bool) {
	raw := ar.Object.Raw
	if ar.Operation == admissionv1.Delete {
		raw = ar.OldObject.Raw
	}
	u := &unstructured.Unstructured{}
	if err := u.UnmarshalJSON(raw); err != nil {
		return "", false
	}
	obj, err := ToLang(u)
	if err != nil {
		return "", false
	}
	info := bsl.NewStructure()
	_ = info.Insert("Операция", operationRu[ar.Operation])
	_ = info.Insert("Пользователь", ar.UserInfo.Username)
	groups := bsl.NewArray()
	for _, g := range ar.UserInfo.Groups {
		groups.Elems = append(groups.Elems, g)
	}
	_ = info.Insert("Группы", groups)
	_ = info.Insert("Подразделение", ar.Namespace)
	var old bsl.Value = bsl.Undefined{}
	if len(ar.OldObject.Raw) > 0 && ar.Operation == admissionv1.Update {
		ou := &unstructured.Unstructured{}
		if ou.UnmarshalJSON(ar.OldObject.Raw) == nil {
			old, _ = ToLang(ou)
		}
	}
	_ = info.Insert("СтарыйОбъект", old)

	r.mu.Lock()
	defer r.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cancelRef := &bsl.Ref{V: false}
	m := r.ob.Module.Method(method)
	args := []*bsl.Ref{{V: obj}, cancelRef, {V: info}}[:min(3, len(m.Params))]
	if err := r.call(ctx, method, args...); err != nil {
		// a broken check must not lock the cluster: allow, and say so
		r.log.Error(err, "проверка заполнения упала, документ пропущен")
		return "", false
	}
	if cancelRef.V == true {
		return r.said("Документ не прошёл проверку заполнения"), true
	}
	return "", false
}

// webhookServer serves the checks of all обработки of one обработчик and
// registers them with the API server under a certificate of its own.
type webhookServer struct {
	opts   WebhookOptions
	c      client.Client
	checks []*check
}

func (s *webhookServer) NeedLeaderElection() bool { return false }

func (s *webhookServer) Start(ctx context.Context) error {
	host := fmt.Sprintf("%s.%s.svc", s.opts.Service, s.opts.Namespace)
	caPEM, cert, err := selfSigned(host)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	for _, c := range s.checks {
		mux.HandleFunc(c.path(), c.admit)
	}
	srv := &http.Server{
		Addr: s.opts.Addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServeTLS("", "") }()
	if err := s.register(ctx, caPEM); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	case err := <-errc:
		return err
	}
}

// register writes the ValidatingWebhookConfiguration. Failures are ignored
// by the API server: an обработчик that is down does not stop the cluster.
func (s *webhookServer) register(ctx context.Context, caPEM []byte) error {
	ignore := admissionregistrationv1.Ignore
	none := admissionregistrationv1.SideEffectClassNone
	timeout := int32(10)
	cfg := &admissionregistrationv1.ValidatingWebhookConfiguration{
		TypeMeta:   metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "ValidatingWebhookConfiguration"},
		ObjectMeta: metav1.ObjectMeta{Name: "obrabotchik-" + s.opts.Namespace + "-" + s.opts.Service},
	}
	for _, c := range s.checks {
		r, rule := c.r, c.rule
		path := c.path()
		var rules []admissionregistrationv1.RuleWithOperations
		for _, k := range rule.kinds {
			m, err := r.cluster.mapper.RESTMapping(k.GroupKind(), k.Version)
			if err != nil {
				return fmt.Errorf("%s: %w", r.ob.Name, err)
			}
			scope := admissionregistrationv1.AllScopes
			rules = append(rules, admissionregistrationv1.RuleWithOperations{
				Operations: rule.ops,
				Rule: admissionregistrationv1.Rule{
					APIGroups: []string{k.Group}, APIVersions: []string{k.Version},
					Resources: []string{m.Resource.Resource}, Scope: &scope,
				},
			})
		}
		cfg.Webhooks = append(cfg.Webhooks, admissionregistrationv1.ValidatingWebhook{
			Name: c.name(),
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service:  &admissionregistrationv1.ServiceReference{Name: s.opts.Service, Namespace: s.opts.Namespace, Path: &path},
				CABundle: caPEM,
			},
			Rules:                   rules,
			FailurePolicy:           &ignore,
			SideEffects:             &none,
			TimeoutSeconds:          &timeout,
			AdmissionReviewVersions: []string{"v1"},
			NamespaceSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key: "kubernetes.io/metadata.name", Operator: metav1.LabelSelectorOpNotIn,
				Values: append(rule.exempt, s.opts.Namespace),
			}}},
		})
	}
	u, err := toUnstructuredTyped(cfg)
	if err != nil {
		return err
	}
	return s.c.Apply(ctx, client.ApplyConfigurationFromUnstructured(u), client.FieldOwner("obrabotchik"), client.ForceOwnership)
}

func toUnstructuredTyped(obj any) (*unstructured.Unstructured, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{}
	return u, u.UnmarshalJSON(b)
}

// selfSigned makes a CA and a serving certificate for host, valid ten
// years: the обработчик makes new ones every start anyway.
func selfSigned(host string) ([]byte, tls.Certificate, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	now := time.Now()
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Удостоверяющий центр обработчика"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(10, 0, 0),
		IsCA: true, KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: host},
		DNSNames:  []string{host, strings.TrimSuffix(host, ".svc"), host + ".cluster.local"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(10, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	caCert, _ := x509.ParseCertificate(caDER)
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, tls.Certificate{}, err
	}
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), cert, err
}

// SetupWebhooks serves the checks of the обработки that have
// ОбработкаПроверкиЗаполнения.
func SetupWebhooks(mgr manager.Manager, runners []*Runner, opts WebhookOptions) error {
	s := &webhookServer{opts: opts, c: mgr.GetClient()}
	for _, r := range runners {
		for _, method := range r.ob.Checks {
			rule, err := r.ob.webhook(method)
			if err != nil {
				return err
			}
			s.checks = append(s.checks, &check{r: r, method: method, rule: rule})
		}
	}
	if len(s.checks) == 0 {
		return nil
	}
	if opts.Service == "" || opts.Namespace == "" {
		return fmt.Errorf("для проверок заполнения нужны --service и --namespace обработчика")
	}
	return mgr.Add(s)
}
