// Command 1c-zaglushka stands in for the 1C:Enterprise server. Linked or
// called as ragent, ras, rac, ibcmd or webinst it behaves as they do; as
// «publikaciya» it serves the web client of a publication.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tym83/assnetes/pkg/zaglushka"
)

func main() {
	name := filepath.Base(os.Args[0])
	args := os.Args[1:]
	if name == "1c-zaglushka" && len(args) > 0 {
		name, args = args[0], args[1:]
	}
	var err error
	switch name {
	case "ragent":
		err = ragent(args)
	case "ras":
		err = ras(args)
	case "rac":
		err = rac(args)
	case "ibcmd":
		err = zaglushka.Ibcmd(args, os.Stdout)
	case "webinst":
		err = webinst(args)
	case "publikaciya":
		err = publikaciya(args)
	default:
		err = fmt.Errorf("заглушка 1С: ragent, ras, rac, ibcmd, webinst или publikaciya, а не «%s»", name)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ragent takes the keys of the real one: -d, -port, -regport, -range.
func ragent(args []string) error {
	fs := flag.NewFlagSet("ragent", flag.ContinueOnError)
	dir := fs.String("d", "/home/usr1cv8/.1cv8/1C/1cv8", "каталог реестра кластера")
	port := fs.Int("port", 1540, "порт агента")
	regport := fs.Int("regport", 1541, "порт менеджера кластера")
	rng := fs.String("range", "1560:1591", "диапазон портов рабочих процессов")
	fs.Int("seclev", 0, "")
	fs.Bool("debug", false, "")
	fs.Bool("http", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	host, _ := os.Hostname()
	if h := os.Getenv("ONEC_HOSTNAME"); h != "" {
		host = h
	}
	if l := os.Getenv("ZAGLUSHKA_LICENZIJ"); l != "" {
		zaglushka.LicenseLimit, _ = strconv.Atoi(l)
	}
	reg, err := zaglushka.OpenRegistry(*dir, host, *port, *regport, *rng)
	if err != nil {
		return err
	}
	fmt.Printf("Агент сервера 1С:Предприятия %s (заглушка) запущен: %s:%d, кластер %s\n", zaglushka.Version, host, *port, reg.Cluster.UUID)
	return http.ListenAndServe(fmt.Sprintf(":%d", *port), (&zaglushka.Agent{Reg: reg}).Handler())
}

// ras: «ras cluster --port=1545 localhost:1540».
func ras(args []string) error {
	port, agent := "1545", "localhost:1540"
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--port="):
			port = strings.TrimPrefix(a, "--port=")
		case a == "cluster" || strings.HasPrefix(a, "-"):
		default:
			agent = a
		}
	}
	fmt.Printf("Сервер администрирования (заглушка) слушает %s, агент %s\n", port, agent)
	return http.ListenAndServe(":"+port, zaglushka.RasProxy(agent))
}

// rac: «rac [host:port] режим команда параметры».
func rac(args []string) error {
	target := "localhost:1545"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") && strings.Contains(args[0], ":") {
		target, args = args[0], args[1:]
	}
	if len(args) > 0 && (args[0] == "help" || args[0] == "--version" || args[0] == "-v") {
		fmt.Printf("rac %s (заглушка Assnetes)\n", zaglushka.Version)
		return nil
	}
	return zaglushka.RacClient(target, args, os.Stdout)
}

// webinst writes default.vrd the way the real one does for Apache.
func webinst(args []string) error {
	opts := map[string]string{}
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			opts[strings.TrimLeft(args[i], "-")] = args[i+1]
			i++
		}
	}
	dir, base, conn := opts["dir"], opts["wsdir"], opts["connstr"]
	if dir == "" || base == "" || conn == "" {
		return fmt.Errorf("webinst: нужны -wsdir, -dir и -connstr")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	vrd := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<point xmlns="http://v8.1c.ru/8.2/virtual-resource-system" base="/%s" ib="%s">
	<ws pointEnableCommon="false"/>
	<httpServices publishByDefault="true"/>
	<standardOdata enable="true" reuseSessions="autouse" sessionMaxAge="20" poolSize="10"/>
</point>
`, base, conn)
	if err := os.WriteFile(filepath.Join(dir, "default.vrd"), []byte(vrd), 0o644); err != nil {
		return err
	}
	fmt.Printf("Публикация «%s» выполнена\n", base)
	return nil
}

func publikaciya(args []string) error {
	fs := flag.NewFlagSet("publikaciya", flag.ContinueOnError)
	port := fs.Int("port", 8080, "порт")
	base := fs.String("base", "/", "адрес публикации")
	agent := fs.String("agent", "localhost:1540", "агент сервера")
	ib := fs.String("infobase", "", "информационная база")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p := &zaglushka.Publication{Agent: *agent, Base: strings.TrimSuffix(*base, "/"), Infobase: *ib}
	fmt.Printf("Публикация %s базы «%s» (заглушка) на :%d\n", p.Base, p.Infobase, *port)
	return http.ListenAndServe(fmt.Sprintf(":%d", *port), p.Handler())
}
