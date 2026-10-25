package zaglushka

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"
	"time"
)

// Publication is the stand-in of a web publication: what Apache with wsap24
// serves for a real platform — a web client that opens a session.
type Publication struct {
	Agent    string // host:port of the agent
	Base     string // «/buh»
	Infobase string
}

var page = template.Must(template.New("").Parse(`<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{if .Config}}{{.Config.Synonym}}{{else}}{{.Infobase}}{{end}} / 1Ж:Предприятие</title>
<style>
:root{--fon:#f2f0e6;--panel:#fbf3d0;--akcent:#e9a400;--temno:#4a4a4a;--zel:#3a8f3a;--krasn:#c0392b}
*{box-sizing:border-box}body{margin:0;font:14px/1.4 Arial,sans-serif;background:var(--fon);color:var(--temno)}
header{display:flex;align-items:center;gap:12px;padding:6px 12px;background:var(--panel);border-bottom:1px solid #d9cfa3}
.logo{font-weight:bold;color:#fff;background:var(--akcent);border-radius:4px;padding:2px 8px}
.title{font-size:15px}.user{margin-left:auto;font-size:13px}
.wrap{display:flex;min-height:calc(100vh - 38px)}
nav{width:200px;background:var(--panel);border-right:1px solid #d9cfa3;padding:8px 0}
nav a{display:block;padding:6px 14px;color:var(--temno);text-decoration:none}nav a:hover{background:#f3e3a0}
main{flex:1;padding:16px}h1{font-size:20px;font-weight:normal;margin:0 0 12px}
.card{background:#fff;border:1px solid #ddd;border-radius:4px;padding:12px;margin-bottom:12px;max-width:720px}
.btn{display:inline-block;background:var(--zel);color:#fff;border-radius:3px;padding:4px 12px;margin-right:6px}
.err{border-left:4px solid var(--krasn)}.muted{color:#888;font-size:12px}
table{border-collapse:collapse}td{padding:2px 12px 2px 0}
@media(max-width:640px){nav{display:none}}
</style></head><body>
<header><span class="logo">1Ж</span><span class="title">{{if .Config}}{{.Config.Synonym}}{{else}}Информационная база «{{.Infobase}}»{{end}}</span>
<span class="user">{{if .Session}}Администратор · сеанс {{.Session.ID}}{{end}}</span></header>
<div class="wrap"><nav>
{{range .Sections}}<a href="#">{{.}}</a>{{end}}
</nav><main>
{{if .Error}}<div class="card err"><h1>1Ж:Предприятие</h1><p>{{.Error}}</p>
<p class="muted">Это заглушка платформы: так же ответил бы настоящий сервер 1С.</p></div>
{{else}}<h1>Начальная страница</h1>
<div class="card"><table>
<tr><td>Информационная база</td><td><b>{{.Infobase}}</b></td></tr>
{{if .Config}}<tr><td>Конфигурация</td><td>{{.Config.Synonym}} ({{.Config.Name}}), редакция {{.Config.Version}}</td></tr>
<tr><td>Поставщик</td><td>{{.Config.Vendor}}</td></tr>{{else}}<tr><td>Конфигурация</td><td>не загружена</td></tr>{{end}}
<tr><td>Сеанс</td><td>{{.Session.ID}}, сервер {{.Session.Server}}</td></tr>
<tr><td>Начат</td><td>{{.Session.StartedAt.Format "02.01.2006 15:04:05"}}</td></tr>
</table></div>
<div class="card"><span class="btn">Провести и закрыть</span><span class="btn">Записать</span>
<p class="muted">Платформа 1С здесь заменена заглушкой Assnetes: сеансы, лицензии, блокировки и конфигурация в СУБД настоящие, формы — нет.</p></div>
{{end}}
</main></div></body></html>`))

var sections = []string{"Главное", "Банк и касса", "Продажи", "Покупки", "Склад", "Производство", "ОС и НМА",
	"Зарплата и кадры", "Операции", "Отчеты", "Справочники", "Администрирование"}

func (p *Publication) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "публикация работает") })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, p.Base) {
			http.Redirect(w, r, p.Base+"/", http.StatusFound)
			return
		}
		data := map[string]any{"Infobase": p.Infobase, "Sections": sections}
		c := http.Client{Timeout: 30 * time.Second}
		var info struct {
			Config *Configuration `json:"config"`
		}
		if resp, err := c.Get("http://" + p.Agent + "/infobase/" + p.Infobase); err == nil {
			json.NewDecoder(resp.Body).Decode(&info)
			resp.Body.Close()
		}
		data["Config"] = info.Config
		sess, err := p.session(w, r)
		if err != nil {
			data["Error"] = err.Error()
		} else {
			data["Session"] = sess
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		page.Execute(w, data)
	})
	return mux
}

// session keeps one session per browser, by a cookie, as the web client does.
func (p *Publication) session(w http.ResponseWriter, r *http.Request) (*Session, error) {
	c := http.Client{Timeout: 30 * time.Second}
	if ck, err := r.Cookie("ibsession"); err == nil {
		if parts := strings.SplitN(ck.Value, "|", 2); len(parts) == 2 {
			resp, err := c.Post("http://"+p.Agent+"/session/"+parts[0]+"/touch", "", nil)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				var s Session
				if json.Unmarshal([]byte(parts[1]), &s) == nil {
					return &s, nil
				}
			}
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	q := "infobase=" + p.Infobase + "&user=" + "Администратор" + "&host=" + host + "&app=WebClient&uc=" + r.URL.Query().Get("uc")
	resp, err := c.Post("http://"+p.Agent+"/session?"+strings.ReplaceAll(q, " ", "%20"), "", nil)
	if err != nil {
		return nil, fmt.Errorf("Ошибка соединения с сервером 1С:Предприятия: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var msg strings.Builder
		buf := make([]byte, 4096)
		n, _ := resp.Body.Read(buf)
		msg.Write(buf[:n])
		return nil, fmt.Errorf("%s", strings.TrimSpace(msg.String()))
	}
	var s Session
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	js, _ := json.Marshal(Session{ID: s.ID, Server: s.Server, StartedAt: s.StartedAt})
	http.SetCookie(w, &http.Cookie{Name: "ibsession", Value: s.UUID + "|" + string(js), Path: p.Base, HttpOnly: true})
	return &s, nil
}
