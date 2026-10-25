package zaglushka

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Version is the version the stand-in reports.
const Version = "8.3.27.9999"

// block writes one object the way rac does: keys padded to the longest one,
// « : », values; strings with spaces in quotes.
func block(b *strings.Builder, pairs [][2]string) {
	w := 0
	for _, p := range pairs {
		w = max(w, len(p[0]))
	}
	for _, p := range pairs {
		fmt.Fprintf(b, "%-*s : %s\n", w, p[0], p[1])
	}
	b.WriteString("\n")
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func yes(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
func itoa[T int | int64](v T) string { return strconv.FormatInt(int64(v), 10) }

type racArgs struct {
	words []string
	opts  map[string]string
}

func parseRac(args []string) racArgs {
	a := racArgs{opts: map[string]string{}}
	for i := 0; i < len(args); i++ {
		s := args[i]
		if !strings.HasPrefix(s, "--") {
			a.words = append(a.words, s)
			continue
		}
		k, v, ok := strings.Cut(strings.TrimPrefix(s, "--"), "=")
		if !ok {
			v = "yes" // a flag such as --create-database
		}
		a.opts[k] = v
	}
	return a
}

func (a racArgs) str(k string) (string, bool) {
	v, ok := a.opts[k]
	return v, ok
}

func (a racArgs) num(k string, into *int64) error {
	v, ok := a.opts[k]
	if !ok {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fmt.Errorf("Неверное значение параметра --%s", k)
	}
	*into = n
	return nil
}

func onOff(v string) bool { return v == "on" || v == "yes" || v == "true" }

// Rac runs a command of rac against the registry and returns its output.
func (r *Registry) Rac(args []string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expire(time.Now())
	a := parseRac(args)
	if len(a.words) == 0 {
		return "", fmt.Errorf("Не указан режим. Используйте rac help")
	}
	mode := a.words[0]
	cmd := ""
	if len(a.words) > 1 {
		cmd = a.words[1]
	}
	if mode != "agent" && !(mode == "cluster" && cmd == "list") {
		c, ok := a.str("cluster")
		if !ok {
			return "", fmt.Errorf("Не указан обязательный параметр --cluster")
		}
		if c != r.Cluster.UUID {
			return "", fmt.Errorf("Ошибка операции администрирования\nКластер %s не найден", c)
		}
	}
	var b strings.Builder
	var err error
	switch mode + " " + cmd {
	case "agent version":
		fmt.Fprintf(&b, "version : %s\n\n", Version)
	case "cluster list", "cluster info":
		r.clusterBlock(&b)
	case "cluster update":
		err = r.clusterUpdate(a)
	case "server list":
		for _, s := range r.Servers {
			serverBlock(&b, s)
		}
	case "server info":
		s := r.server(a.opts["server"])
		if s == nil {
			return "", fmt.Errorf("Ошибка операции администрирования\nРабочий сервер не найден")
		}
		serverBlock(&b, *s)
	case "server insert":
		err = r.serverInsert(&b, a)
	case "server update":
		s := r.server(a.opts["server"])
		if s == nil {
			return "", fmt.Errorf("Ошибка операции администрирования\nРабочий сервер не найден")
		}
		err = serverSet(s, a)
	case "server remove":
		err = r.serverRemove(a.opts["server"])
	case "infobase summary", "infobase info", "infobase create", "infobase update", "infobase drop":
		err = r.infobaseCmd(&b, a)
	case "session list":
		ib := a.opts["infobase"]
		for _, s := range r.Sessions {
			if ib == "" || s.Infobase == ib {
				r.sessionBlock(&b, s)
			}
		}
	case "session terminate":
		err = r.sessionTerminate(a.opts["session"])
	case "process list":
		for _, s := range r.Servers {
			if srv, ok := a.opts["server"]; ok && srv != s.UUID {
				continue
			}
			r.processBlock(&b, s)
		}
	case "rule list":
		for _, rule := range r.Rules {
			if rule.Server == a.opts["server"] {
				ruleBlock(&b, rule)
			}
		}
	case "rule insert":
		err = r.ruleInsert(&b, a)
	case "rule apply":
	case "service list":
		for _, s := range services {
			block(&b, [][2]string{{"name", s[0]}, {"main-only", s[1]}, {"servers", ""}, {"descr", quote(s[2])}})
		}
	default:
		return "", fmt.Errorf("Неизвестная команда: %s %s", mode, cmd)
	}
	if err != nil {
		return "", err
	}
	return b.String(), r.saveLocked()
}

var services = [][3]string{
	{"EventLogService", "0", "Сервис журналов регистрации"},
	{"LicenseService", "0", "Сервис лицензирования"},
	{"SessionDataService", "0", "Сервис сеансовых данных"},
	{"JobService", "0", "Сервис заданий"},
	{"TransactionLockService", "0", "Сервис транзакционных блокировок"},
	{"FulltextSearchService", "0", "Сервис полнотекстового поиска"},
	{"DbCopiesService", "0", "Сервис копий баз данных"},
}

func (r *Registry) clusterBlock(b *strings.Builder) {
	c := r.Cluster
	block(b, [][2]string{
		{"cluster", c.UUID}, {"host", c.Host}, {"port", itoa(c.Port)}, {"name", quote(c.Name)},
		{"expiration-timeout", itoa(c.ExpirationTimeout)}, {"lifetime-limit", itoa(c.LifetimeLimit)},
		{"max-memory-size", itoa(c.MaxMemorySize)}, {"max-memory-time-limit", "0"},
		{"security-level", itoa(c.SecurityLevel)}, {"session-fault-tolerance-level", itoa(c.SessionFaultToleranceLevel)},
		{"load-balancing-mode", c.LoadBalancingMode}, {"errors-count-threshold", "0"},
		{"kill-problem-processes", itoa(c.KillProblemProcesses)}, {"kill-by-memory-with-dump", "0"},
	})
}

func (r *Registry) clusterUpdate(a racArgs) error {
	if v, ok := a.str("load-balancing-mode"); ok {
		r.Cluster.LoadBalancingMode = v
	}
	var n int64 = int64(r.Cluster.SessionFaultToleranceLevel)
	if err := a.num("session-fault-tolerance-level", &n); err != nil {
		return err
	}
	r.Cluster.SessionFaultToleranceLevel = int(n)
	return a.num("max-memory-size", &r.Cluster.MaxMemorySize)
}

func serverBlock(b *strings.Builder, s Server) {
	block(b, [][2]string{
		{"server", s.UUID}, {"agent-host", s.AgentHost}, {"agent-port", itoa(s.AgentPort)},
		{"port-range", s.PortRange}, {"name", quote(s.Name)}, {"using", s.Using},
		{"dedicate-managers", s.DedicateManagers}, {"infobases-limit", itoa(s.InfobasesLimit)},
		{"memory-limit", itoa(s.MemoryLimit)}, {"connections-limit", itoa(s.ConnectionsLimit)},
		{"safe-working-processes-memory-limit", itoa(s.SafeWorkingProcessesMemoryLimit)},
		{"safe-call-memory-limit", itoa(s.SafeCallMemoryLimit)}, {"cluster-port", itoa(s.ClusterPort)},
		{"critical-total-memory", itoa(s.CriticalTotalMemory)},
		{"temporary-allowed-total-memory", itoa(s.TemporaryAllowedTotalMemory)},
		{"temporary-allowed-total-memory-time-limit", itoa(s.TemporaryAllowedTotalMemoryTimeLimit)},
		{"service-principal-name", `""`},
	})
}

func serverSet(s *Server, a racArgs) error {
	for k, into := range map[string]*int64{
		"safe-working-processes-memory-limit":       &s.SafeWorkingProcessesMemoryLimit,
		"safe-call-memory-limit":                    &s.SafeCallMemoryLimit,
		"critical-total-memory":                     &s.CriticalTotalMemory,
		"temporary-allowed-total-memory":            &s.TemporaryAllowedTotalMemory,
		"temporary-allowed-total-memory-time-limit": &s.TemporaryAllowedTotalMemoryTimeLimit,
		"memory-limit":                              &s.MemoryLimit,
	} {
		if err := a.num(k, into); err != nil {
			return err
		}
	}
	for k, into := range map[string]*int{"infobases-limit": &s.InfobasesLimit, "connections-limit": &s.ConnectionsLimit} {
		n := int64(*into)
		if err := a.num(k, &n); err != nil {
			return err
		}
		*into = int(n)
	}
	if v, ok := a.str("name"); ok {
		s.Name = v
	}
	if v, ok := a.str("using"); ok {
		s.Using = v
	}
	if v, ok := a.str("dedicate-managers"); ok {
		s.DedicateManagers = v
	}
	return nil
}

func (r *Registry) serverInsert(b *strings.Builder, a racArgs) error {
	host, ok := a.str("agent-host")
	if !ok {
		return fmt.Errorf("Не указан обязательный параметр --agent-host")
	}
	for _, s := range r.Servers {
		if s.AgentHost == host {
			return fmt.Errorf("Ошибка операции администрирования\nРабочий сервер %s уже зарегистрирован в кластере", host)
		}
	}
	var port int64 = 1540
	if err := a.num("agent-port", &port); err != nil {
		return err
	}
	pr := a.opts["port-range"]
	if pr == "" {
		pr = "1560:1591"
	}
	name := a.opts["name"]
	if name == "" {
		name = host
	}
	s := defaultServer(host, int(port), r.Cluster.Port, pr, name, "normal")
	if err := serverSet(&s, a); err != nil {
		return err
	}
	r.Servers = append(r.Servers, s)
	block(b, [][2]string{{"server", s.UUID}})
	return nil
}

func (r *Registry) serverRemove(id string) error {
	for i, s := range r.Servers {
		if s.UUID != id {
			continue
		}
		if s.Using == "main" && i == 0 {
			return fmt.Errorf("Ошибка операции администрирования\nНельзя удалить центральный сервер, на котором работает кластер")
		}
		r.Servers = append(r.Servers[:i], r.Servers[i+1:]...)
		kept := r.Sessions[:0]
		for _, sess := range r.Sessions {
			if sess.Server != id {
				kept = append(kept, sess)
			}
		}
		r.Sessions = kept
		rules := r.Rules[:0]
		for _, rule := range r.Rules {
			if rule.Server != id {
				rules = append(rules, rule)
			}
		}
		r.Rules = rules
		return nil
	}
	return fmt.Errorf("Ошибка операции администрирования\nРабочий сервер не найден")
}

func (r *Registry) infobaseCmd(b *strings.Builder, a racArgs) error {
	cmd := a.words[1]
	if cmd == "summary" {
		for _, ib := range r.Infobases {
			block(b, [][2]string{{"infobase", ib.UUID}, {"name", ib.Name}, {"descr", quote(ib.Descr)}})
		}
		return nil
	}
	if cmd == "create" {
		name := a.opts["name"]
		if name == "" {
			return fmt.Errorf("Не указан обязательный параметр --name")
		}
		if r.infobase(name) != nil {
			return fmt.Errorf("Ошибка операции администрирования\nИнформационная база с именем %s уже зарегистрирована в кластере", name)
		}
		ib := Infobase{
			UUID: newUUID(), Name: name, Descr: a.opts["descr"], DBMS: a.opts["dbms"], DBServer: a.opts["db-server"],
			DBName: a.opts["db-name"], DBUser: a.opts["db-user"], DBPwd: a.opts["db-pwd"], Locale: a.opts["locale"],
			LicenseDistrib: a.opts["license-distribution"],
		}
		if _, ok := a.str("create-database"); ok {
			if err := createDatabase(ib); err != nil {
				return err
			}
		}
		r.Infobases = append(r.Infobases, ib)
		block(b, [][2]string{{"infobase", ib.UUID}})
		return nil
	}
	ib := r.infobase(a.opts["infobase"])
	if ib == nil {
		return fmt.Errorf("Ошибка операции администрирования\nИнформационная база не найдена")
	}
	switch cmd {
	case "info":
		block(b, [][2]string{
			{"infobase", ib.UUID}, {"name", ib.Name}, {"dbms", ib.DBMS}, {"db-server", ib.DBServer},
			{"db-name", ib.DBName}, {"db-user", ib.DBUser}, {"security-level", "0"},
			{"license-distribution", orDefault(ib.LicenseDistrib, "allow")},
			{"scheduled-jobs-deny", yes(ib.ScheduledJobsDeny)}, {"sessions-deny", yes(ib.SessionsDeny)},
			{"denied-from", ib.DeniedFrom}, {"denied-message", quote(ib.DeniedMessage)},
			{"denied-parameter", `""`}, {"denied-to", ib.DeniedTo}, {"permission-code", quote(ib.PermissionCode)},
			{"external-session-manager-connection-string", `""`}, {"external-session-manager-required", "no"},
			{"security-profile-name", `""`}, {"safe-mode-security-profile-name", `""`},
			{"reserve-working-processes", "no"}, {"descr", quote(ib.Descr)},
		})
	case "update":
		if v, ok := a.str("sessions-deny"); ok {
			ib.SessionsDeny = onOff(v)
		}
		if v, ok := a.str("scheduled-jobs-deny"); ok {
			ib.ScheduledJobsDeny = onOff(v)
		}
		for k, into := range map[string]*string{"denied-message": &ib.DeniedMessage, "permission-code": &ib.PermissionCode,
			"denied-from": &ib.DeniedFrom, "denied-to": &ib.DeniedTo, "descr": &ib.Descr} {
			if v, ok := a.str(k); ok {
				*into = v
			}
		}
	case "drop":
		if _, ok := a.str("drop-database"); ok {
			if err := dropDatabase(*ib); err != nil {
				return err
			}
		}
		for i := range r.Infobases {
			if r.Infobases[i].UUID == ib.UUID {
				r.Infobases = append(r.Infobases[:i], r.Infobases[i+1:]...)
				break
			}
		}
	}
	return nil
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func (r *Registry) sessionBlock(b *strings.Builder, s Session) {
	block(b, [][2]string{
		{"session", s.UUID}, {"session-id", itoa(s.ID)}, {"infobase", s.Infobase},
		{"connection", "00000000-0000-0000-0000-000000000000"}, {"process", processUUID(s.Server)},
		{"user-name", s.UserName}, {"host", s.Host}, {"app-id", s.AppID}, {"locale", "ru"},
		{"started-at", s.StartedAt.Format("2006-01-02T15:04:05")},
		{"last-active-at", s.LastActiveAt.Format("2006-01-02T15:04:05")}, {"hibernate", "no"},
		{"passive-session-hibernate-time", "1200"}, {"hibernate-session-terminate-time", "86400"},
		{"blocked-by-dbms", "0"}, {"blocked-by-ls", "0"},
	})
}

func (r *Registry) sessionTerminate(id string) error {
	for i, s := range r.Sessions {
		if s.UUID == id {
			r.Sessions = append(r.Sessions[:i], r.Sessions[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("Ошибка операции администрирования\nСеанс не найден")
}

// processUUID is the working process of a server: one per server here.
func processUUID(server string) string {
	if len(server) < 8 {
		return "00000000-0000-0000-0000-000000000000"
	}
	return "0" + server[1:]
}

func (r *Registry) processBlock(b *strings.Builder, s Server) {
	n := r.sessionsOn(s.UUID)
	start := 1560
	if a, _, ok := strings.Cut(s.PortRange, ":"); ok {
		start, _ = strconv.Atoi(a)
	}
	block(b, [][2]string{
		{"process", processUUID(s.UUID)}, {"host", s.AgentHost}, {"port", itoa(start + 1)},
		{"pid", itoa(1000 + len(s.AgentHost))}, {"is-enable", "yes"}, {"running", "yes"},
		{"started-at", r.Started.Format("2006-01-02T15:04:05")}, {"use", "used"},
		{"available-perfomance", itoa(max(5, 100-n*12))}, {"capacity", "1000"},
		{"connections", itoa(n)}, {"memory-size", itoa(int64(180000 + 45000*n))},
		{"memory-excess-time", "0"}, {"selection-size", "0"}, {"avg-call-time", "0.008"},
		{"avg-db-call-time", "0.002"}, {"avg-lock-call-time", "0.000"}, {"avg-server-call-time", "0.004"},
		{"avg-threads", "0.001"}, {"reserve", "no"},
	})
}

func ruleBlock(b *strings.Builder, r Rule) {
	block(b, [][2]string{
		{"rule", r.UUID}, {"object-type", r.ObjectType}, {"infobase-name", r.InfobaseName},
		{"rule-type", r.RuleType}, {"application-ext", r.AppExt}, {"priority", itoa(r.Priority)},
	})
}

func (r *Registry) ruleInsert(b *strings.Builder, a racArgs) error {
	srv := a.opts["server"]
	if r.server(srv) == nil {
		return fmt.Errorf("Ошибка операции администрирования\nРабочий сервер не найден")
	}
	rule := Rule{UUID: newUUID(), Server: srv, ObjectType: a.opts["object-type"], InfobaseName: a.opts["infobase-name"],
		RuleType: orDefault(a.opts["rule-type"], "auto"), AppExt: a.opts["application-ext"]}
	var n int64
	_ = a.num("priority", &n)
	rule.Priority = int(n)
	_ = a.num("position", &n)
	rule.Position = int(n)
	r.Rules = append(r.Rules, rule)
	sort.SliceStable(r.Rules, func(i, j int) bool { return r.Rules[i].Position < r.Rules[j].Position })
	block(b, [][2]string{{"rule", rule.UUID}})
	return nil
}

// LicenseLimit is how many sessions the licenses allow; the developer
// license of 1C allows three.
var LicenseLimit = 3

// StartSession starts a session with an infobase on the server with the
// least sessions, the way the cluster balances them.
func (r *Registry) StartSession(infobase, user, host, app, code string) (Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.expire(now)
	ib := r.infobase(infobase)
	if ib == nil {
		return Session{}, fmt.Errorf("Информационная база не обнаружена (%s)", infobase)
	}
	if ib.SessionsDeny && (ib.PermissionCode == "" || code != ib.PermissionCode) {
		return Session{}, fmt.Errorf("Начало сеанса с информационной базой запрещено. %s", ib.DeniedMessage)
	}
	if LicenseLimit > 0 && len(r.Sessions) >= LicenseLimit {
		return Session{}, fmt.Errorf("Нет свободной лицензии на рабочее место. Занято лицензий: %d из %d", len(r.Sessions), LicenseLimit)
	}
	var best *Server
	for i := range r.Servers {
		s := r.Servers[i]
		if !r.serverAllowed(s, ib.Name) {
			continue
		}
		if best == nil || r.sessionsOn(s.UUID) < r.sessionsOn(best.UUID) {
			best = &r.Servers[i]
		}
	}
	if best == nil {
		return Session{}, fmt.Errorf("Нет доступных рабочих процессов. Требования назначения функциональности не позволяют обслужить сеанс")
	}
	s := Session{UUID: newUUID(), ID: r.NextID, Infobase: ib.UUID, Server: best.UUID, UserName: user, Host: host,
		AppID: app, StartedAt: now, LastActiveAt: now}
	r.NextID++
	r.Sessions = append(r.Sessions, s)
	return s, r.saveLocked()
}

// Touch keeps a session alive; false when it is gone.
func (r *Registry) Touch(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.Sessions {
		if r.Sessions[i].UUID == id {
			r.Sessions[i].LastActiveAt = time.Now()
			return true
		}
	}
	return false
}

// InfobaseByName returns a copy of an infobase.
func (r *Registry) InfobaseByName(name string) (Infobase, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ib := r.infobase(name)
	if ib == nil {
		return Infobase{}, false
	}
	return *ib, true
}
