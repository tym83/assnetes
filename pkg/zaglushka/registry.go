// Package zaglushka is a stand-in for the 1C:Enterprise server: ragent,
// ras, rac, ibcmd and webinst that answer the way the documentation says
// the real ones answer. It lets the operators of the distribution be tested
// end to end without 1C, whose binaries cannot be redistributed. Infobases
// are real databases in PostgreSQL; the cluster, its servers, sessions and
// working processes are a registry kept by the stand-in ragent.
package zaglushka

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cluster is a cluster of 1C servers.
type Cluster struct {
	UUID                       string `json:"uuid"`
	Host                       string `json:"host"`
	Port                       int    `json:"port"`
	Name                       string `json:"name"`
	ExpirationTimeout          int    `json:"expirationTimeout"`
	LifetimeLimit              int    `json:"lifetimeLimit"`
	MaxMemorySize              int64  `json:"maxMemorySize"`
	SecurityLevel              int    `json:"securityLevel"`
	SessionFaultToleranceLevel int    `json:"sessionFaultToleranceLevel"`
	LoadBalancingMode          string `json:"loadBalancingMode"`
	KillProblemProcesses       int    `json:"killProblemProcesses"`
}

// Server is a working server of a cluster.
type Server struct {
	UUID                                 string `json:"uuid"`
	AgentHost                            string `json:"agentHost"`
	AgentPort                            int    `json:"agentPort"`
	PortRange                            string `json:"portRange"`
	Name                                 string `json:"name"`
	Using                                string `json:"using"`
	DedicateManagers                     string `json:"dedicateManagers"`
	InfobasesLimit                       int    `json:"infobasesLimit"`
	MemoryLimit                          int64  `json:"memoryLimit"`
	ConnectionsLimit                     int    `json:"connectionsLimit"`
	SafeWorkingProcessesMemoryLimit      int64  `json:"safeWorkingProcessesMemoryLimit"`
	SafeCallMemoryLimit                  int64  `json:"safeCallMemoryLimit"`
	ClusterPort                          int    `json:"clusterPort"`
	CriticalTotalMemory                  int64  `json:"criticalTotalMemory"`
	TemporaryAllowedTotalMemory          int64  `json:"temporaryAllowedTotalMemory"`
	TemporaryAllowedTotalMemoryTimeLimit int64  `json:"temporaryAllowedTotalMemoryTimeLimit"`
}

// Infobase is an infobase registered in the cluster.
type Infobase struct {
	UUID              string `json:"uuid"`
	Name              string `json:"name"`
	Descr             string `json:"descr"`
	DBMS              string `json:"dbms"`
	DBServer          string `json:"dbServer"`
	DBName            string `json:"dbName"`
	DBUser            string `json:"dbUser"`
	DBPwd             string `json:"dbPwd"`
	Locale            string `json:"locale"`
	SessionsDeny      bool   `json:"sessionsDeny"`
	DeniedMessage     string `json:"deniedMessage"`
	DeniedFrom        string `json:"deniedFrom"`
	DeniedTo          string `json:"deniedTo"`
	PermissionCode    string `json:"permissionCode"`
	ScheduledJobsDeny bool   `json:"scheduledJobsDeny"`
	LicenseDistrib    string `json:"licenseDistribution"`
}

// Session is a session of a user with an infobase.
type Session struct {
	UUID         string    `json:"uuid"`
	ID           int       `json:"id"`
	Infobase     string    `json:"infobase"`
	Server       string    `json:"server"`
	UserName     string    `json:"userName"`
	Host         string    `json:"host"`
	AppID        string    `json:"appId"`
	StartedAt    time.Time `json:"startedAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
}

// Rule is a requirement of functionality assignment.
type Rule struct {
	UUID         string `json:"uuid"`
	Server       string `json:"server"`
	Position     int    `json:"position"`
	ObjectType   string `json:"objectType"`
	InfobaseName string `json:"infobaseName"`
	RuleType     string `json:"ruleType"`
	AppExt       string `json:"appExt"`
	Priority     int    `json:"priority"`
}

// Registry is the cluster registry, what srvinfo holds for a real ragent.
type Registry struct {
	mu        sync.Mutex
	path      string
	Cluster   Cluster    `json:"cluster"`
	Servers   []Server   `json:"servers"`
	Infobases []Infobase `json:"infobases"`
	Sessions  []Session  `json:"sessions"`
	Rules     []Rule     `json:"rules"`
	NextID    int        `json:"nextSessionId"`
	Started   time.Time  `json:"started"`
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// OpenRegistry reads the registry from dir, or makes a cluster with one
// central server on this host, as the first start of a real ragent does.
func OpenRegistry(dir, host string, port, regport int, portRange string) (*Registry, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	r := &Registry{path: filepath.Join(dir, "1CV8Clst.json")}
	data, err := os.ReadFile(r.path)
	if err == nil {
		if err := json.Unmarshal(data, r); err != nil {
			return nil, fmt.Errorf("реестр кластера повреждён: %w", err)
		}
		r.Started = time.Now()
		r.Sessions = nil // sessions do not survive a restart of the cluster
		return r, r.saveLocked()
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	r.Cluster = Cluster{
		UUID: newUUID(), Host: host, Port: regport, Name: "Локальный кластер",
		ExpirationTimeout: 60, LoadBalancingMode: "performance", KillProblemProcesses: 1,
	}
	r.Servers = []Server{defaultServer(host, port, regport, portRange, "Центральный сервер", "main")}
	r.NextID = 1
	r.Started = time.Now()
	return r, r.saveLocked()
}

func defaultServer(host string, port, regport int, portRange, name, using string) Server {
	return Server{
		UUID: newUUID(), AgentHost: host, AgentPort: port, PortRange: portRange, Name: name,
		Using: using, DedicateManagers: "none", InfobasesLimit: 8, ConnectionsLimit: 256, ClusterPort: regport,
	}
}

func (r *Registry) saveLocked() error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

func (r *Registry) infobase(name string) *Infobase {
	for i := range r.Infobases {
		if r.Infobases[i].Name == name || r.Infobases[i].UUID == name {
			return &r.Infobases[i]
		}
	}
	return nil
}

func (r *Registry) server(id string) *Server {
	for i := range r.Servers {
		if r.Servers[i].UUID == id || r.Servers[i].Name == id || r.Servers[i].AgentHost == id {
			return &r.Servers[i]
		}
	}
	return nil
}

// expire drops sessions idle longer than the cluster's timeout for them.
func (r *Registry) expire(now time.Time) {
	kept := r.Sessions[:0]
	for _, s := range r.Sessions {
		if now.Sub(s.LastActiveAt) < 20*time.Minute {
			kept = append(kept, s)
		}
	}
	r.Sessions = kept
}

// serverAllowed reports whether a server may take sessions of an infobase,
// by the requirements of functionality assignment.
func (r *Registry) serverAllowed(s Server, ib string) bool {
	decided := ""
	for _, rule := range r.Rules {
		if rule.Server != s.UUID || rule.ObjectType != "" && rule.ObjectType != "SessionDataService" && rule.ObjectType != "Client" {
			continue
		}
		if rule.InfobaseName != "" && rule.InfobaseName != ib {
			continue
		}
		decided = rule.RuleType
	}
	return decided != "never"
}

// processesOf counts sessions per server.
func (r *Registry) sessionsOn(server string) int {
	n := 0
	for _, s := range r.Sessions {
		if s.Server == server {
			n++
		}
	}
	return n
}
