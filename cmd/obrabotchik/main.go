// Command obrabotchik runs operators written in the 1C language: each
// argument is the directory of one обработка.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	_ "time/tzdata"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/tym83/assnetes/pkg/obrabotka"
)

func main() {
	var leader, webhooks bool
	var metrics, probes, id, whAddr, whService, whNamespace string
	flag.BoolVar(&leader, "monopolnyj-rezhim", true, "монопольный режим: работает один экземпляр, остальные ждут (leader election)")
	flag.StringVar(&metrics, "metrics", ":8080", "адрес метрик")
	flag.StringVar(&probes, "probes", ":8081", "адрес проб живости и готовности")
	flag.StringVar(&id, "id", "obrabotchik", "имя блокировки монопольного режима")
	flag.BoolVar(&obrabotka.RunScheduleAtStart, "srazu", false, "выполнить регламентные задания сразу при старте")
	flag.BoolVar(&webhooks, "proverki", true, "проверки заполнения (admission webhooks)")
	flag.StringVar(&whAddr, "webhook-addr", ":9443", "адрес проверок заполнения")
	flag.StringVar(&whService, "service", "", "Service обработчика: через него API-сервер зовёт проверки заполнения")
	flag.StringVar(&whNamespace, "namespace", os.Getenv("POD_NAMESPACE"), "подразделение обработчика")
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	log := ctrl.Log.WithName("обработчик")

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "Укажите каталоги обработок. Без первички не запущусь.")
		os.Exit(2)
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		LeaderElection:         leader,
		LeaderElectionID:       id,
		Metrics:                metricsserver.Options{BindAddress: metrics},
		HealthProbeBindAddress: probes,
	})
	if err != nil {
		log.Error(err, "менеджер не создан")
		os.Exit(1)
	}
	ctx := ctrl.SetupSignalHandler()
	var runners []*obrabotka.Runner
	for _, dir := range flag.Args() {
		ob, err := obrabotka.Load(dir)
		if err != nil {
			log.Error(err, "обработка не загружена", "каталог", dir)
			os.Exit(1)
		}
		if err := obrabotka.InstallKinds(ctx, mgr.GetConfig(), ob); err != nil {
			log.Error(err, "виды обработки не установлены", "обработка", ob.Name)
			os.Exit(1)
		}
		r, err := obrabotka.Setup(mgr, ob)
		if err != nil {
			log.Error(err, "обработка не подключена", "обработка", ob.Name)
			os.Exit(1)
		}
		runners = append(runners, r)
		log.Info("обработка подключена", "обработка", ob.Name, "вид", ob.Kind.String(), "регламентных", len(ob.Schedule))
	}
	if !webhooks {
		log.Info("проверки заполнения выключены")
	} else if err := obrabotka.SetupWebhooks(mgr, runners, obrabotka.WebhookOptions{Addr: whAddr, Service: whService, Namespace: whNamespace}); err != nil {
		log.Error(err, "проверки заполнения не подключены")
		os.Exit(1)
	}
	_ = mgr.AddHealthzCheck("kofta-na-stule", func(_ *http.Request) error { return nil })
	_ = mgr.AddReadyzCheck("na-meste", func(_ *http.Request) error { return nil })
	if err := mgr.Start(ctx); err != nil {
		log.Error(err, "обработчик остановлен")
		os.Exit(1)
	}
}
