package zaglushka

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Agent serves the registry: rac commands, sessions of web clients, and
// the configuration of infobases for publications.
type Agent struct {
	Reg *Registry
}

func (a *Agent) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rac", func(w http.ResponseWriter, r *http.Request) {
		var args []string
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&args); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out, err := a.Reg.Rac(args)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			io.WriteString(w, err.Error()+"\n")
			return
		}
		io.WriteString(w, out)
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s, err := a.Reg.StartSession(q.Get("infobase"), q.Get("user"), q.Get("host"), q.Get("app"), q.Get("uc"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		json.NewEncoder(w).Encode(s)
	})
	mux.HandleFunc("POST /session/{id}/touch", func(w http.ResponseWriter, r *http.Request) {
		if !a.Reg.Touch(r.PathValue("id")) {
			http.Error(w, "Сеанс завершён администратором", http.StatusGone)
		}
	})
	mux.HandleFunc("GET /infobase/{name}", func(w http.ResponseWriter, r *http.Request) {
		ib, ok := a.Reg.InfobaseByName(r.PathValue("name"))
		if !ok {
			http.Error(w, "Информационная база не обнаружена", http.StatusNotFound)
			return
		}
		info := map[string]any{"name": ib.Name, "sessionsDeny": ib.SessionsDeny, "deniedMessage": ib.DeniedMessage}
		if cf, ok, err := ReadConfig(ib, "main"); err == nil && ok {
			info["config"] = cf
		} else if err != nil {
			info["error"] = err.Error()
		}
		json.NewEncoder(w).Encode(info)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ragent работает\n") })
	return mux
}

// RacClient sends a command of rac to an agent: the stand-in's ras is a
// proxy to the agent, so rac may name either.
func RacClient(target string, args []string, out io.Writer) error {
	body, _ := json.Marshal(args)
	c := http.Client{Timeout: 60 * time.Second}
	resp, err := c.Post("http://"+target+"/rac", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("Ошибка соединения с сервером администрирования %s: %v", target, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", strings.TrimSpace(string(data)))
	}
	_, err = out.Write(data)
	return err
}

// RasProxy is the administration server: it forwards rac to the agent.
func RasProxy(agent string) http.Handler {
	u, _ := url.Parse("http://" + agent)
	return httputil.NewSingleHostReverseProxy(u)
}
