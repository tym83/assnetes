package cli

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/tym83/assnetes/pkg/bsl"
	"github.com/tym83/assnetes/pkg/golos"
	"github.com/tym83/assnetes/pkg/manifest"
	"github.com/tym83/assnetes/pkg/perevod"
	"github.com/tym83/assnetes/pkg/zapros"
)

//go:embed gui
var guiFS embed.FS

func newGuiCommand(o *options) *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:     "гуи",
		Aliases: []string{"gui", "предприятие"},
		Short:   "запустить 1Ж:Предприятие — управление кластерами в привычном бухгалтеру окне",
		RunE: func(cmd *cobra.Command, _ []string) error {
			g := &gui{o: o, clients: map[string]*clients{}}
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			fmt.Fprintf(o.out, "1Ж:Предприятие запущено: http://%s/\n", ln.Addr())
			o.say("privetstvie", nil)
			srv := &http.Server{Handler: g.handler(), ReadHeaderTimeout: 10 * time.Second}
			go func() {
				<-cmd.Context().Done()
				srv.Close()
			}()
			err = srv.Serve(ln)
			if err == http.ErrServerClosed {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVar(&addr, "адрес", "127.0.0.1:8383", "где слушать; наружу — на свой страх и риск, входа по паролю нет")
	return cmd
}

type gui struct {
	o       *options
	mu      sync.Mutex
	clients map[string]*clients
}

func (g *gui) handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(guiFS, "gui")
	mux.Handle("GET /", http.FileServer(http.FS(static)))
	mux.HandleFunc("GET /api/bazy", g.bazy)
	mux.HandleFunc("GET /api/vidy", g.vidy)
	mux.HandleFunc("GET /api/{baza}/spisok/{vid}", g.spisok)
	mux.HandleFunc("GET /api/{baza}/obekt/{vid}/{ns}/{imya}", g.obekt)
	mux.HandleFunc("POST /api/{baza}/provesti", g.provesti)
	mux.HandleFunc("POST /api/{baza}/pometit/{vid}/{ns}/{imya}", g.pometit)
	mux.HandleFunc("POST /api/{baza}/zapros", g.zapros)
	mux.HandleFunc("GET /api/{baza}/zhurnal", g.zhurnal)
	mux.HandleFunc("GET /golos/{situaciya}", g.golos)
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

// fail answers with the error worded the way 1C words it, and the original.
func (g *gui) fail(w http.ResponseWriter, err error, v golos.Vars) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	code := http.StatusBadRequest
	var st apierrors.APIStatus
	if s, ok := err.(apierrors.APIStatus); ok {
		st = s
		code = int(st.Status().Code)
	}
	if code == 0 {
		code = http.StatusBadRequest
	}
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"ошибка":    golos.New(false).Error(err, v),
		"подробнее": err.Error(),
	})
}

// bazy is the list of infobases: the contexts of the kubeconfig.
func (g *gui) bazy(w http.ResponseWriter, _ *http.Request) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if g.o.kubeconfig != "" {
		rules.ExplicitPath = g.o.kubeconfig
	}
	cfg, err := rules.Load()
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	type baza struct {
		Имя, Сервер string
		Текущая     bool
	}
	var out []baza
	for name, c := range cfg.Contexts {
		server := ""
		if cl, ok := cfg.Clusters[c.Cluster]; ok {
			server = cl.Server
		}
		out = append(out, baza{Имя: name, Сервер: server, Текущая: name == cfg.CurrentContext})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Имя < out[j].Имя })
	writeJSON(w, out)
}

// vidy is the plan of kinds the sections of the interface show.
func (g *gui) vidy(w http.ResponseWriter, _ *http.Request) {
	type vid struct {
		Вид, Множественное, API string
		ВПодразделении          bool
	}
	var out []vid
	for _, k := range perevod.Kinds {
		out = append(out, vid{Вид: k.Ru, Множественное: k.Plural, API: k.Kind, ВПодразделении: k.Namespaced})
	}
	writeJSON(w, out)
}

func (g *gui) client(name string) (*clients, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.clients[name]; ok {
		return c, nil
	}
	o := *g.o
	o.context = name
	o.clients = nil
	c, err := o.client()
	if err != nil {
		return nil, err
	}
	g.clients[name] = c
	return c, nil
}

func (g *gui) spisok(w http.ResponseWriter, r *http.Request) {
	c, err := g.client(r.PathValue("baza"))
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	m, kind, err := c.resolveKind(r.PathValue("vid"))
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	ns := ""
	if p := r.URL.Query().Get("podrazdelenie"); p != "" && m.Scope.Name() == meta.RESTScopeNameNamespace {
		ns = perevod.Name(p)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	list, err := c.resource(m, ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		g.fail(w, err, golos.Vars{"Вид": r.PathValue("vid")})
		return
	}
	header, rows := tableFor(m.GroupVersionKind.Kind, list.Items, m.Scope.Name() == meta.RESTScopeNameNamespace)
	type stroka struct {
		Ячейки        []string
		Имя, ИмяAPI   string
		Подразделение string
		Пометка       bool
	}
	var out []stroka
	byName := map[string]*unstructured.Unstructured{}
	for i := range list.Items {
		byName[list.Items[i].GetNamespace()+"/"+displayName(&list.Items[i])] = &list.Items[i]
	}
	for _, row := range rows {
		s := stroka{Ячейки: row}
		if m.Scope.Name() == meta.RESTScopeNameNamespace {
			s.Подразделение, s.Имя = row[0], row[1]
		} else {
			s.Имя = row[0]
		}
		if u := byName[s.Подразделение+"/"+s.Имя]; u != nil {
			s.ИмяAPI = u.GetName()
			_, s.Пометка = u.GetAnnotations()[AnnMarked]
		}
		out = append(out, s)
	}
	title := m.GroupVersionKind.Kind
	if kind != nil {
		title = kind.Plural
	}
	writeJSON(w, map[string]any{"Заголовок": title, "Колонки": header, "Строки": out})
}

func (g *gui) obekt(w http.ResponseWriter, r *http.Request) {
	c, err := g.client(r.PathValue("baza"))
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	m, _, err := c.resolveKind(r.PathValue("vid"))
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	ns := r.PathValue("ns")
	if ns == "-" {
		ns = ""
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	u, err := c.get(ctx, m, ns, r.PathValue("imya"))
	if err != nil {
		g.fail(w, err, golos.Vars{"Вид": r.PathValue("vid"), "Имя": r.PathValue("imya")})
		return
	}
	v, _ := fromUnstructured(u)
	ru, err := perevod.FromAPI(v)
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	src, _ := manifest.Export(ru)
	js, _ := bsl.WriteJSON(ru, "  ")
	writeJSON(w, map[string]any{
		"Объект":   json.RawMessage(js),
		"Манифест": src,
		"Движения": g.dvizheniya(ctx, c, u),
	})
}

// dvizheniya are «движения документа»: the objects this one owns, the way
// a posted document has entries in registers.
func (g *gui) dvizheniya(ctx context.Context, c *clients, owner *unstructured.Unstructured) []map[string]string {
	var out []map[string]string
	for _, kind := range []string{"Развёртывание", "НаборСоСостоянием", "НаборРеплик", "Под", "Услуга", "Задание", "Справочник", "Секрет", "ЗаявкаНаТом", "Проходная"} {
		m, k, err := c.resolveKind(kind)
		if err != nil || m.Scope.Name() != meta.RESTScopeNameNamespace {
			continue
		}
		ns := owner.GetNamespace()
		if ns == "" {
			continue
		}
		list, err := c.resource(m, ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for i := range list.Items {
			for _, ref := range list.Items[i].GetOwnerReferences() {
				if ref.UID == owner.GetUID() {
					out = append(out, map[string]string{"Вид": k.Ru, "Имя": displayName(&list.Items[i]), "Подразделение": ns})
				}
			}
		}
	}
	return out
}

func (g *gui) provesti(w http.ResponseWriter, r *http.Request) {
	c, err := g.client(r.PathValue("baza"))
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	res, err := manifest.Run(ctx, "форма.bsl", string(body), manifest.Options{Message: func(string) {}})
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	var done []string
	for _, obj := range res.Objects {
		u, err := toUnstructured(obj)
		if err != nil {
			g.fail(w, err, nil)
			return
		}
		m, kind, err := c.mappingOf(u)
		if err != nil {
			g.fail(w, err, nil)
			return
		}
		if m.Scope.Name() == meta.RESTScopeNameNamespace && u.GetNamespace() == "" {
			u.SetNamespace(c.namespace)
		}
		if _, err := c.resource(m, u.GetNamespace()).Apply(ctx, u.GetName(), u, applyOptions); err != nil {
			g.fail(w, err, golos.Vars{"Вид": u.GetKind(), "Имя": perevod.DisplayName(obj)})
			return
		}
		name := u.GetKind()
		if kind != nil {
			name = kind.Ru
		}
		done = append(done, fmt.Sprintf("%s «%s» %s", name, perevod.DisplayName(obj), agree(kind, "проведён", "проведена", "проведено", "проведены")))
	}
	_, phrase := golos.New(false).Say("proveden", golos.Vars{"Вид": "Документ", "Имя": strings.Join(done, ", ")})
	writeJSON(w, map[string]any{"Проведено": done, "Фраза": phrase})
}

func (g *gui) pometit(w http.ResponseWriter, r *http.Request) {
	c, err := g.client(r.PathValue("baza"))
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	m, _, err := c.resolveKind(r.PathValue("vid"))
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	ns := r.PathValue("ns")
	if ns == "-" {
		ns = ""
	}
	value := `"` + time.Now().UTC().Format(time.RFC3339) + `"`
	if r.URL.Query().Get("snyat") != "" {
		value = "null"
	}
	patch := []byte(`{"metadata":{"annotations":{"` + AnnMarked + `":` + value + `}}}`)
	_, err = c.resource(m, ns).Patch(r.Context(), r.PathValue("imya"), "application/merge-patch+json", patch, metav1.PatchOptions{FieldManager: fieldManager})
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	_, phrase := golos.New(false).Say("pomechen", golos.Vars{"Вид": r.PathValue("vid"), "Имя": r.PathValue("imya")})
	writeJSON(w, map[string]string{"Фраза": phrase})
}

func (g *gui) zapros(w http.ResponseWriter, r *http.Request) {
	c, err := g.client(r.PathValue("baza"))
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	q, err := zapros.Parse(string(body))
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	t, err := zapros.Execute(q, &clusterData{ctx: ctx, c: c, cache: map[string][]bsl.Value{}}, nil)
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	var rows [][]string
	for _, row := range t.Rows() {
		var cells []string
		for _, col := range t.Columns() {
			cells = append(cells, bsl.ToString(row.Value(col)))
		}
		rows = append(rows, cells)
	}
	writeJSON(w, map[string]any{"Колонки": t.Columns(), "Строки": rows})
}

// zhurnal is the event log: events of the cluster, newest first, with the
// importance 1C shows.
func (g *gui) zhurnal(w http.ResponseWriter, r *http.Request) {
	c, err := g.client(r.PathValue("baza"))
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	list, err := c.typed.CoreV1().Events("").List(r.Context(), metav1.ListOptions{Limit: 300})
	if err != nil {
		g.fail(w, err, nil)
		return
	}
	type zapis struct {
		Дата, Важность, Событие, Метаданные, Данные, Комментарий string
	}
	var out []zapis
	for _, e := range list.Items {
		t := e.LastTimestamp.Time
		if t.IsZero() {
			t = e.EventTime.Time
		}
		vazhnost := "Информация"
		if e.Type == "Warning" {
			vazhnost = "Ошибка"
		}
		kind := e.InvolvedObject.Kind
		if k, ok := perevod.LookupKind(kind); ok {
			kind = k.Ru
		}
		out = append(out, zapis{
			Дата: t.Local().Format("02.01.2006 15:04:05"), Важность: vazhnost, Событие: e.Reason,
			Метаданные: kind, Данные: e.InvolvedObject.Namespace + "/" + e.InvolvedObject.Name, Комментарий: e.Message,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ti, _ := time.Parse("02.01.2006 15:04:05", out[i].Дата)
		tj, _ := time.Parse("02.01.2006 15:04:05", out[j].Дата)
		return ti.After(tj)
	})
	writeJSON(w, out)
}

func (g *gui) golos(w http.ResponseWriter, r *http.Request) {
	persona, phrase := golos.New(false).Say(r.PathValue("situaciya"), golos.Vars{})
	cal := golos.New(false)
	extra := ""
	if s := cal.Calendar(); s != "" {
		_, extra = cal.Say(s, nil)
	}
	writeJSON(w, map[string]any{"Персона": persona, "Фраза": phrase, "Календарь": extra, "Обед": cal.Lunch()})
}
