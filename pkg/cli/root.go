// Package cli is 1сctl, the command line of the distribution: kubectl that
// takes and gives everything in Russian and talks like an accountant.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/tym83/assnetes/pkg/golos"
)

// Version is set at build time.
var Version = "0.0.0-dev"

type options struct {
	kubeconfig  string
	context     string
	namespace   string
	allNS       bool
	output      string
	quiet       bool
	director    bool
	out, errOut io.Writer
	tty         bool
	golos       *golos.Golos
	clients     *clients
}

// NewRootCommand builds 1сctl.
func NewRootCommand() *cobra.Command {
	o := &options{out: os.Stdout, errOut: os.Stderr}
	o.tty = term.IsTerminal(int(os.Stdout.Fd()))
	root := &cobra.Command{
		Use:           "1сctl",
		Short:         "1Ж:Оркестратор 8 — управление кластером по-бухгалтерски",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			quiet := o.quiet || !o.tty || os.Getenv("CI") != "" || (o.output != "" && o.output != "таблица")
			o.golos = golos.New(quiet)
		},
	}
	f := root.PersistentFlags()
	f.StringVar(&o.kubeconfig, "kubeconfig", "", "файл со списком информационных баз (kubeconfig)")
	f.StringVar(&o.context, "контекст", "", "кластер из списка информационных баз")
	f.StringVar(&o.namespace, "подразделение", "", "подразделение (namespace)")
	f.BoolVar(&o.allNS, "все-подразделения", false, "по всем подразделениям")
	f.StringVar(&o.output, "вывод", "", "таблица, bsl, yaml, json или api (YAML как в API, без перевода)")
	f.BoolVar(&o.quiet, "тихо", false, "без комментариев бухгалтерии")
	f.BoolVar(&o.director, "я-директор", false, "без очереди и без обеда")
	// English spellings for muscle memory and scripts.
	f.StringVar(&o.context, "context", "", "")
	f.StringVarP(&o.namespace, "namespace", "n", "", "")
	f.BoolVarP(&o.allNS, "all-namespaces", "A", false, "")
	f.StringVarP(&o.output, "output", "o", "", "")
	for _, n := range []string{"context", "namespace", "all-namespaces", "output"} {
		_ = f.MarkHidden(n)
	}

	root.AddCommand(
		newApplyCommand(o),
		newGetCommand(o),
		newDescribeCommand(o),
		newExportCommand(o),
		newDeleteCommand(o),
		newMarkCommand(o),
		newLogsCommand(o),
		newKindsCommand(o),
		newRunCommand(o),
		newVvodCommand(o),
		newKonfiguratorCommand(o),
		newZaprosCommand(o),
		newOSVCommand(o),
		&cobra.Command{
			Use:     "версия",
			Aliases: []string{"version"},
			Short:   "версия платформы",
			Run: func(*cobra.Command, []string) {
				fmt.Fprintf(o.out, "1Ж:Оркестратор 8 (1сctl) %s\n", Version)
			},
		},
	)
	newPaketyCommand(root)
	return root
}

// Execute runs 1сctl and returns the exit code.
func Execute() int {
	cmd := NewRootCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, red(isTTY(os.Stderr), "Ошибка: ")+err.Error())
		return 1
	}
	return 0
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

func paint(on bool, code, s string) string {
	if !on {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func red(on bool, s string) string   { return paint(on, "31", s) }
func green(on bool, s string) string { return paint(on, "32", s) }
func dim(on bool, s string) string   { return paint(on, "2", s) }
func bold(on bool, s string) string  { return paint(on, "1", s) }

// say prints a phrase of the department, dimmed, if anyone says one.
func (o *options) say(situation string, v golos.Vars) {
	persona, phrase := o.golos.Say(situation, v)
	if phrase == "" {
		return
	}
	fmt.Fprintln(o.out, dim(o.tty, "  — "+phrase+" ("+persona+")"))
}

// calendar adds what the calendar has to say, once per command.
func (o *options) calendar() {
	if s := o.golos.Calendar(); s != "" {
		o.say(s, nil)
	}
	if o.golos.Lunch() && !o.director {
		o.say("obed", nil)
	}
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
