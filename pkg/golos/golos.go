// Package golos is the voice of the accounting department: what the CLI,
// the GUI and the operators say to people, by situation, persona and the
// accounting calendar.
package golos

import (
	"bytes"
	_ "embed"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

//go:embed frazy.yaml
var frazyYAML []byte

type catalog struct {
	situations map[string]map[string][]string
	errors     map[string]string
}

func load() catalog {
	var raw map[string]yaml.Node
	if err := yaml.Unmarshal(frazyYAML, &raw); err != nil {
		panic(fmt.Sprintf("frazy.yaml: %v", err))
	}
	c := catalog{situations: map[string]map[string][]string{}}
	for k, n := range raw {
		if k == "oshibki" {
			if err := n.Decode(&c.errors); err != nil {
				panic(err)
			}
			continue
		}
		var m map[string][]string
		if err := n.Decode(&m); err != nil {
			panic(fmt.Sprintf("frazy.yaml %s: %v", k, err))
		}
		c.situations[k] = m
	}
	return c
}

var frazy = load()

// Golos speaks. Quiet makes it say nothing extra: scripts, pipes, CI.
type Golos struct {
	Quiet bool
	Now   func() time.Time
	rnd   *rand.Rand
}

func New(quiet bool) *Golos {
	return &Golos{Quiet: quiet, Now: time.Now, rnd: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 83))}
}

// Vars are what phrases substitute.
type Vars map[string]any

func render(text string, v Vars) string {
	t, err := template.New("").Option("missingkey=zero").Parse(text)
	if err != nil {
		return text
	}
	var b bytes.Buffer
	if err := t.Execute(&b, map[string]any(v)); err != nil {
		return text
	}
	return strings.ReplaceAll(b.String(), "<no value>", "")
}

// Say returns a phrase for a situation, with the persona who says it.
// It returns empty strings when quiet or when nobody says anything.
func (g *Golos) Say(situation string, v Vars) (persona, phrase string) {
	if g.Quiet {
		return "", ""
	}
	byPersona := frazy.situations[situation]
	if len(byPersona) == 0 {
		return "", ""
	}
	personas := make([]string, 0, len(byPersona))
	for p := range byPersona {
		personas = append(personas, p)
	}
	sort.Strings(personas)
	persona = personas[g.rnd.IntN(len(personas))]
	lines := byPersona[persona]
	return persona, render(lines[g.rnd.IntN(len(lines))], v)
}

// Calendar returns the situation the accounting calendar adds today, if any.
func (g *Golos) Calendar() string {
	now := g.Now()
	switch {
	case now.Month() == time.November && now.Day() == 21:
		return "den-buhgaltera"
	case now.Day() == 25:
		return "dvadcat-pyatoe"
	case now.Day() == 28:
		return "dvadcat-vosmoe"
	case now.Day() <= 7 && workdaysSinceMonthStart(now) <= 5:
		return "zakrytie-mesyaca"
	}
	return ""
}

// Lunch reports whether the department is at lunch now.
func (g *Golos) Lunch() bool {
	h := g.Now().Hour()
	return h == 12
}

func workdaysSinceMonthStart(t time.Time) int {
	n := 0
	for d := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()); !d.After(t); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			n++
		}
	}
	return n
}

// ErrorKind classifies an error of the API the way the catalog keys them.
func ErrorKind(err error) string {
	if _, ok := deniedBy(err); ok {
		return "otkaz"
	}
	switch {
	case apierrors.IsConflict(err):
		return "conflict"
	case apierrors.IsAlreadyExists(err):
		return "exists"
	case apierrors.IsForbidden(err):
		if strings.Contains(err.Error(), "exceeded quota") {
			return "quota"
		}
		return "forbidden"
	case apierrors.IsUnauthorized(err):
		return "unauthorized"
	case apierrors.IsNotFound(err):
		return "notfound"
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
		return "invalid"
	case apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return "timeout"
	case apierrors.IsInternalError(err) && strings.Contains(err.Error(), "typed patch"):
		return "invalid" // server-side apply rejects fields the schema does not know
	case apierrors.IsServiceUnavailable(err), apierrors.IsInternalError(err):
		return "unavailable"
	}
	return "other"
}

// deniedBy extracts what an admission check said when it refused.
func deniedBy(err error) (string, bool) {
	msg := err.Error()
	const marker = "denied the request: "
	if i := strings.Index(msg, marker); i >= 0 && strings.Contains(msg, "admission webhook") {
		return msg[i+len(marker):], true
	}
	return "", false
}

// Error words an error of the API the way the 1C platform would. The
// original message is the caller's to show next to it.
func (g *Golos) Error(err error, v Vars) string {
	kind := ErrorKind(err)
	if v == nil {
		v = Vars{}
	}
	if reason, ok := deniedBy(err); ok {
		v["Причина"] = reason
	} else if _, ok := v["Причина"]; !ok {
		v["Причина"] = err.Error()
	}
	return render(frazy.errors[kind], v)
}
