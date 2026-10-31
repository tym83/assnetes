package obrabotka

// Выписка: what the books of BukhOps give Flux. A ledger's balances,
// rendered into objects of the cluster, are packed into a tarball, filed
// in a ConfigMap next to the ledger, announced as a Flux ExternalArtifact
// and served over HTTP by every replica of the обработчик, so that
// kustomize-controller can fetch it whichever replica leads.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/yaml"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/perevod"
)

// VypiskiBase is the address kustomize-controller fetches выписки from:
// the Service of the обработчик, «http://obrabotchik.assnetes.svc:9090».
// Empty, the обработчик publishes nothing.
var VypiskiBase string

var externalArtifact = schema.GroupVersionKind{Group: "source.toolkit.fluxcd.io", Version: "v1", Kind: "ExternalArtifact"}

const (
	vypiskaKey    = "vypiska.tar.gz"
	annDigest     = perevod.Group + "/dajdzhest"
	vypiskaPrefix = "/vypiska/"
)

func vypiskaConfigMap(ledger string) string { return ledger + "-vypiska" }

// Pack writes objects of the API as YAML files into a gzipped tarball that
// is the same, byte for byte, for the same objects: the revision of the
// books must not change when the books did not.
func Pack(objs []*unstructured.Unstructured) ([]byte, error) {
	type file struct {
		name string
		data []byte
	}
	files := make([]file, 0, len(objs))
	seen := map[string]bool{}
	for _, u := range objs {
		u = u.DeepCopy()
		delete(u.Object, "status")
		data, err := yaml.Marshal(u.Object)
		if err != nil {
			return nil, err
		}
		ns := u.GetNamespace()
		if ns == "" {
			ns = "_"
		}
		name := strings.ToLower(fmt.Sprintf("%s-%s-%s.yaml", u.GetKind(), ns, u.GetName()))
		if seen[name] {
			return nil, fmt.Errorf("документ %s %s/%s в выписке дважды", u.GetKind(), ns, u.GetName())
		}
		seen[name] = true
		files = append(files, file{name, data})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(gz)
	for _, f := range files {
		hdr := &tar.Header{
			Name: f.name, Mode: 0o644, Size: int64(len(f.data)), Typeflag: tar.TypeReg,
			ModTime: time.Unix(0, 0), Format: tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// publish is ОпубликоватьВыписку(Книга, Документы, Метка): it files the
// выписка of a ledger and announces it to Flux. It returns
// Структура(Ревизия, Адрес, Дайджест, Размер, Новая).
func (cl *Cluster) publish(ledgerV, docsV bsl.Value, label string) (bsl.Value, error) {
	if VypiskiBase == "" {
		return nil, fmt.Errorf("обработчик запущен без адреса выписок (--vypiski-addr и --service)")
	}
	ledger, err := FromLang(ledgerV)
	if err != nil {
		return nil, err
	}
	ns, name := ledger.GetNamespace(), ledger.GetName()
	if ledger.GetUID() == "" {
		live := &unstructured.Unstructured{}
		live.SetGroupVersionKind(ledger.GroupVersionKind())
		if err := cl.c.Get(cl.ctx, client.ObjectKey{Namespace: ns, Name: name}, live); err != nil {
			return nil, fmt.Errorf("книга: %w", err)
		}
		ledger = live
	}
	var objs []*unstructured.Unstructured
	if it, ok := docsV.(bsl.Iterable); ok {
		for _, d := range it.Items() {
			u, err := FromLang(d)
			if err != nil {
				return nil, err
			}
			objs = append(objs, u)
		}
	}
	data, err := Pack(objs)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	digest := "sha256:" + hexSum
	if label == "" {
		label = "nachalnye-ostatki"
	}
	revision := perevod.Name(label) + "@" + digest
	path := ns + "/" + name + "/" + hexSum + ".tar.gz"
	url := strings.TrimSuffix(VypiskiBase, "/") + vypiskaPrefix + path

	yes := true
	owner := []metav1.OwnerReference{{
		APIVersion: ledger.GetAPIVersion(), Kind: ledger.GetKind(), Name: name, UID: ledger.GetUID(),
		Controller: &yes, BlockOwnerDeletion: &yes,
	}}

	cm := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{
			"name": vypiskaConfigMap(name), "namespace": ns,
			"annotations": map[string]any{annDigest: digest},
		},
		"binaryData": map[string]any{vypiskaKey: data},
	}}
	cm.SetOwnerReferences(owner)
	if err := cl.c.Apply(cl.ctx, client.ApplyConfigurationFromUnstructured(cm), client.FieldOwner(cl.manager), client.ForceOwnership); err != nil {
		return nil, fmt.Errorf("выписка не подшита: %w", err)
	}

	ea := &unstructured.Unstructured{}
	ea.SetGroupVersionKind(externalArtifact)
	ea.SetName(name)
	ea.SetNamespace(ns)
	ea.SetOwnerReferences(owner)
	_ = unstructured.SetNestedMap(ea.Object, map[string]any{
		"apiVersion": ledger.GetAPIVersion(), "kind": ledger.GetKind(), "name": name,
	}, "spec", "sourceRef")
	if err := cl.c.Apply(cl.ctx, client.ApplyConfigurationFromUnstructured(ea), client.FieldOwner(cl.manager), client.ForceOwnership); err != nil {
		if meta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("Flux не установлен: в кластере нет вида ExternalArtifact (нужен Flux 2.7 или новее)")
		}
		return nil, fmt.Errorf("выписка не объявлена: %w", err)
	}

	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(externalArtifact)
	if err := cl.c.Get(cl.ctx, client.ObjectKey{Namespace: ns, Name: name}, live); err != nil {
		return nil, err
	}
	fresh := true
	if old, _, _ := unstructured.NestedString(live.Object, "status", "artifact", "revision"); old == revision {
		fresh = false
	}
	if fresh || !ready(live) {
		now := metav1.Now().UTC().Format(time.RFC3339)
		live.Object["status"] = map[string]any{
			"observedGeneration": live.GetGeneration(),
			"artifact": map[string]any{
				"digest": digest, "lastUpdateTime": now, "path": path,
				"revision": revision, "size": int64(len(data)), "url": url,
			},
			"conditions": []any{map[string]any{
				"type": "Ready", "status": "True", "reason": "Succeeded",
				"message":            "stored artifact for revision '" + revision + "'",
				"lastTransitionTime": now, "observedGeneration": live.GetGeneration(),
			}},
		}
		if err := cl.c.Status().Update(cl.ctx, live); err != nil {
			return nil, fmt.Errorf("состояние выписки не записано: %w", err)
		}
	}

	res := bsl.NewStructure()
	_ = res.Insert("Ревизия", revision)
	_ = res.Insert("Адрес", url)
	_ = res.Insert("Дайджест", digest)
	_ = res.Insert("Размер", bsl.NewInt(int64(len(data))))
	_ = res.Insert("Документов", bsl.NewInt(int64(len(objs))))
	_ = res.Insert("Новая", fresh)
	return res, nil
}

func ready(u *unstructured.Unstructured) bool {
	conds, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, c := range conds {
		m, _ := c.(map[string]any)
		if m["type"] == "Ready" && m["status"] == "True" {
			return true
		}
	}
	return false
}

// vypiskiServer serves выписки to Flux from their ConfigMaps, on every
// replica: the leader files them, any replica hands them out.
type vypiskiServer struct {
	addr   string
	reader client.Reader
}

func (s *vypiskiServer) NeedLeaderElection() bool { return false }

func (s *vypiskiServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc(vypiskaPrefix, s.serve)
	srv := &http.Server{Addr: s.addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	case err := <-errc:
		return err
	}
}

// serve answers GET /vypiska/<namespace>/<ledger>/<sha256>.tar.gz.
func (s *vypiskiServer) serve(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		http.Error(w, "только GET", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.TrimPrefix(req.URL.Path, vypiskaPrefix), "/")
	if len(parts) != 3 || !strings.HasSuffix(parts[2], ".tar.gz") {
		http.NotFound(w, req)
		return
	}
	want := "sha256:" + strings.TrimSuffix(parts[2], ".tar.gz")
	cm := &corev1.ConfigMap{}
	err := s.reader.Get(req.Context(), client.ObjectKey{Namespace: parts[0], Name: vypiskaConfigMap(parts[1])}, cm)
	if apierrors.IsNotFound(err) {
		http.NotFound(w, req)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	data, ok := cm.BinaryData[vypiskaKey]
	if !ok || cm.Annotations[annDigest] != want {
		// a выписка superseded by a newer one is not kept
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	_, _ = w.Write(data)
}

// SetupVypiski serves выписки at addr; base is how others reach it.
func SetupVypiski(mgr manager.Manager, addr, base string) error {
	VypiskiBase = base
	return mgr.Add(&vypiskiServer{addr: addr, reader: mgr.GetAPIReader()})
}

// registerVypiska gives modules ОпубликоватьВыписку.
func registerVypiska(it *bsl.Interp, cl *Cluster) {
	it.Define(func(_ *bsl.Interp, args []*bsl.Ref) (bsl.Value, error) {
		return cl.publish(bsl.Arg(args, 0), bsl.Arg(args, 1), bsl.ToString(bsl.Arg(args, 2)))
	}, "ОпубликоватьВыписку", "PublishArtifact")
}
