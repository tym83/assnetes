// Package perevod translates Kubernetes objects between the Cyrillic form
// people write and read and the form the API stores: kinds, groups and
// fields by a dictionary, names and label values by transliteration with
// the original kept in an annotation.
package perevod

import "strings"

// Group is the API group of the distribution, «оркестратор.бух».
const (
	GroupRu = "оркестратор.бух"
	Group   = "xn--80ajopdiccfkc.xn--90a0bg"
)

// Annotations that keep what transliteration loses.
const (
	AnnName   = Group + "/naimenovanie" // the name as written
	AnnLabels = Group + "/metki"        // label keys and values as written, JSON
	AnnNames  = Group + "/imena"        // other names inside the object as written, JSON
)

// Kind is one kind of object and its Russian names.
type Kind struct {
	Kind       string
	APIVersion string
	Ru         string   // canonical Russian name
	Plural     string   // Russian plural for the CLI
	Aliases    []string // other accepted Russian names and plurals
	Resource   string   // plural resource of the API
	Namespaced bool
	// Native is true for kinds of the distribution, whose fields are
	// Cyrillic in the API itself and are never translated.
	Native bool
	// Gender of the Russian name: 'm', 'f', 'n' or 'p' for plural only,
	// for words that agree with it: «Услуга проведена».
	Gender rune
}

// Agree picks the form of a word that agrees with the kind:
// k.Agree("проведён", "проведена", "проведено", "проведены").
func (k *Kind) Agree(m, f, n, p string) string {
	switch k.Gender {
	case 'f':
		return f
	case 'n':
		return n
	case 'p':
		return p
	}
	return m
}

var Kinds = []Kind{
	{Kind: "Pod", APIVersion: "v1", Ru: "Под", Plural: "Поды", Resource: "pods", Namespaced: true, Gender: 'm'},
	{Kind: "Deployment", APIVersion: "apps/v1", Ru: "Развёртывание", Plural: "Развёртывания", Aliases: []string{"Развертывание", "Развертывания"}, Resource: "deployments", Namespaced: true, Gender: 'n'},
	{Kind: "StatefulSet", APIVersion: "apps/v1", Ru: "НаборСоСостоянием", Plural: "НаборыСоСостоянием", Resource: "statefulsets", Namespaced: true, Gender: 'm'},
	{Kind: "DaemonSet", APIVersion: "apps/v1", Ru: "НаборНаКаждомУзле", Plural: "НаборыНаКаждомУзле", Resource: "daemonsets", Namespaced: true, Gender: 'm'},
	{Kind: "ReplicaSet", APIVersion: "apps/v1", Ru: "НаборРеплик", Plural: "НаборыРеплик", Resource: "replicasets", Namespaced: true, Gender: 'm'},
	{Kind: "Job", APIVersion: "batch/v1", Ru: "Задание", Plural: "Задания", Resource: "jobs", Namespaced: true, Gender: 'n'},
	{Kind: "CronJob", APIVersion: "batch/v1", Ru: "РегламентноеЗадание", Plural: "РегламентныеЗадания", Resource: "cronjobs", Namespaced: true, Gender: 'n'},
	{Kind: "Service", APIVersion: "v1", Ru: "Услуга", Plural: "Услуги", Aliases: []string{"Сервис", "Сервисы"}, Resource: "services", Namespaced: true, Gender: 'f'},
	{Kind: "Ingress", APIVersion: "networking.k8s.io/v1", Ru: "Проходная", Plural: "Проходные", Aliases: []string{"Вход", "Входы"}, Resource: "ingresses", Namespaced: true, Gender: 'f'},
	{Kind: "NetworkPolicy", APIVersion: "networking.k8s.io/v1", Ru: "СетеваяПолитика", Plural: "СетевыеПолитики", Resource: "networkpolicies", Namespaced: true, Gender: 'f'},
	{Kind: "ConfigMap", APIVersion: "v1", Ru: "Справочник", Plural: "Справочники", Resource: "configmaps", Namespaced: true, Gender: 'm'},
	{Kind: "Secret", APIVersion: "v1", Ru: "Секрет", Plural: "Секреты", Aliases: []string{"БланкСтрогойОтчётности"}, Resource: "secrets", Namespaced: true, Gender: 'm'},
	{Kind: "Namespace", APIVersion: "v1", Ru: "Подразделение", Plural: "Подразделения", Aliases: []string{"ОбластьИмён", "ОбластиИмён"}, Resource: "namespaces", Gender: 'n'},
	{Kind: "Node", APIVersion: "v1", Ru: "Узел", Plural: "Узлы", Aliases: []string{"Склад", "Склады"}, Resource: "nodes", Gender: 'm'},
	{Kind: "PersistentVolumeClaim", APIVersion: "v1", Ru: "ЗаявкаНаТом", Plural: "ЗаявкиНаТом", Resource: "persistentvolumeclaims", Namespaced: true, Gender: 'f'},
	{Kind: "PersistentVolume", APIVersion: "v1", Ru: "Том", Plural: "Тома", Resource: "persistentvolumes", Gender: 'm'},
	{Kind: "StorageClass", APIVersion: "storage.k8s.io/v1", Ru: "КлассХранения", Plural: "КлассыХранения", Resource: "storageclasses", Gender: 'm'},
	{Kind: "ServiceAccount", APIVersion: "v1", Ru: "ПодотчётноеЛицо", Plural: "ПодотчётныеЛица", Aliases: []string{"ПодотчетноеЛицо", "ПодотчетныеЛица"}, Resource: "serviceaccounts", Namespaced: true, Gender: 'n'},
	{Kind: "Role", APIVersion: "rbac.authorization.k8s.io/v1", Ru: "Роль", Plural: "Роли", Resource: "roles", Namespaced: true, Gender: 'f'},
	{Kind: "ClusterRole", APIVersion: "rbac.authorization.k8s.io/v1", Ru: "РольКластера", Plural: "РолиКластера", Resource: "clusterroles", Gender: 'f'},
	{Kind: "RoleBinding", APIVersion: "rbac.authorization.k8s.io/v1", Ru: "НазначениеРоли", Plural: "НазначенияРолей", Resource: "rolebindings", Namespaced: true, Gender: 'n'},
	{Kind: "ClusterRoleBinding", APIVersion: "rbac.authorization.k8s.io/v1", Ru: "НазначениеРолиКластера", Plural: "НазначенияРолейКластера", Resource: "clusterrolebindings", Gender: 'n'},
	{Kind: "ResourceQuota", APIVersion: "v1", Ru: "Лимит", Plural: "Лимиты", Resource: "resourcequotas", Namespaced: true, Gender: 'm'},
	{Kind: "LimitRange", APIVersion: "v1", Ru: "НормыРасхода", Plural: "НормыРасходов", Resource: "limitranges", Namespaced: true, Gender: 'p'},
	{Kind: "HorizontalPodAutoscaler", APIVersion: "autoscaling/v2", Ru: "СезонныйНайм", Plural: "СезонныеНаймы", Resource: "horizontalpodautoscalers", Namespaced: true, Gender: 'm'},
	{Kind: "PodDisruptionBudget", APIVersion: "policy/v1", Ru: "ГрафикОтпусков", Plural: "ГрафикиОтпусков", Resource: "poddisruptionbudgets", Namespaced: true, Gender: 'm'},
	{Kind: "Event", APIVersion: "v1", Ru: "Событие", Plural: "События", Resource: "events", Namespaced: true, Gender: 'n'},
	{Kind: "CustomResourceDefinition", APIVersion: "apiextensions.k8s.io/v1", Ru: "РасширениеКонфигурации", Plural: "РасширенияКонфигурации", Resource: "customresourcedefinitions", Gender: 'n'},

	// Kinds of the distribution.
	{Kind: "Platform", APIVersion: Group + "/v1", Ru: "Поставка", Plural: "Поставки", Resource: "platforms", Native: true, Gender: 'f'},
	{Kind: "ServerCluster", APIVersion: Group + "/v1", Ru: "КластерСерверов", Plural: "КластерыСерверов", Resource: "serverclusters", Namespaced: true, Native: true, Gender: 'm'},
	{Kind: "Infobase", APIVersion: Group + "/v1", Ru: "ИнформационнаяБаза", Plural: "ИнформационныеБазы", Aliases: []string{"ИБ"}, Resource: "infobases", Namespaced: true, Native: true, Gender: 'f'},
	{Kind: "Publication", APIVersion: Group + "/v1", Ru: "Публикация", Plural: "Публикации", Resource: "publications", Namespaced: true, Native: true, Gender: 'f'},
	{Kind: "Standalone", APIVersion: Group + "/v1", Ru: "АвтономныйСервер", Plural: "АвтономныеСерверы", Resource: "standalones", Namespaced: true, Native: true, Gender: 'm'},
	{Kind: "InfobaseBackup", APIVersion: Group + "/v1", Ru: "Выгрузка", Plural: "Выгрузки", Resource: "infobasebackups", Namespaced: true, Native: true, Gender: 'f'},
	{Kind: "ConfigurationUpdate", APIVersion: Group + "/v1", Ru: "ОбновлениеКонфигурации", Plural: "ОбновленияКонфигурации", Resource: "configurationupdates", Namespaced: true, Native: true, Gender: 'n'},
	{Kind: "ExchangeLink", APIVersion: Group + "/v1", Ru: "ПланОбмена", Plural: "ПланыОбмена", Resource: "exchangelinks", Namespaced: true, Native: true, Gender: 'm'},
	{Kind: "BitrixSite", APIVersion: Group + "/v1", Ru: "Сайт", Plural: "Сайты", Aliases: []string{"СайтБитрикс"}, Resource: "bitrixsites", Namespaced: true, Native: true, Gender: 'm'},
	{Kind: "ProductionCalendar", APIVersion: Group + "/v1", Ru: "ПроизводственныйКалендарь", Plural: "ПроизводственныеКалендари", Resource: "productioncalendars", Native: true, Gender: 'm'},
	{Kind: "CurrencyRates", APIVersion: Group + "/v1", Ru: "КурсыВалют", Plural: "КурсыВалют", Resource: "currencyrates", Native: true, Gender: 'p'},
	{Kind: "ReconciliationAct", APIVersion: Group + "/v1", Ru: "АктСверки", Plural: "АктыСверки", Resource: "reconciliationacts", Native: true, Gender: 'm'},
}

// field pairs: English, Russian. One Russian word may stand for one field
// only; the English side may repeat in different places of the schema.
var fieldPairs = [][2]string{
	{"apiVersion", "ВерсияAPI"}, {"kind", "Вид"}, {"metadata", "Метаданные"}, {"spec", "Спецификация"},
	{"status", "Состояние"}, {"name", "Имя"}, {"namespace", "Подразделение"}, {"labels", "Метки"},
	{"annotations", "Аннотации"}, {"uid", "Идентификатор"}, {"resourceVersion", "ВерсияРесурса"},
	{"generation", "Поколение"}, {"creationTimestamp", "ДатаСоздания"}, {"deletionTimestamp", "ДатаПометкиНаУдаление"},
	{"ownerReferences", "Владельцы"}, {"finalizers", "Финализаторы"},
	{"replicas", "Реплики"}, {"selector", "Отбор"}, {"matchLabels", "ПоМеткам"}, {"matchExpressions", "ПоВыражениям"},
	{"template", "Шаблон"}, {"containers", "Контейнеры"}, {"initContainers", "НачальныеКонтейнеры"},
	{"image", "Образ"}, {"imagePullPolicy", "ПолитикаЗагрузкиОбраза"}, {"command", "Команда"}, {"args", "Аргументы"},
	{"workingDir", "РабочийКаталог"}, {"ports", "Порты"}, {"containerPort", "ПортКонтейнера"}, {"protocol", "Протокол"},
	{"port", "Порт"}, {"targetPort", "ЦелевойПорт"}, {"nodePort", "ПортУзла"},
	{"env", "Окружение"}, {"envFrom", "ОкружениеИз"}, {"value", "Значение"}, {"valueFrom", "ЗначениеИз"},
	{"secretKeyRef", "СсылкаНаКлючСекрета"}, {"configMapKeyRef", "СсылкаНаКлючСправочника"}, {"key", "Ключ"},
	{"resources", "Ресурсы"}, {"requests", "Запросы"}, {"limits", "Пределы"}, {"cpu", "Процессор"}, {"memory", "Память"},
	{"volumes", "Тома"}, {"volumeMounts", "ПодключенияТомов"}, {"mountPath", "ПутьПодключения"}, {"subPath", "Подпуть"},
	{"readOnly", "ТолькоЧтение"}, {"persistentVolumeClaim", "ЗаявкаНаТом"}, {"claimName", "ИмяЗаявки"},
	{"configMap", "Справочник"}, {"secret", "Секрет"}, {"secretName", "ИмяСекрета"}, {"emptyDir", "ПустойКаталог"},
	{"type", "Тип"}, {"data", "Данные"}, {"stringData", "СтроковыеДанные"}, {"binaryData", "ДвоичныеДанные"},
	{"restartPolicy", "ПолитикаПерезапуска"}, {"nodeSelector", "ОтборУзлов"}, {"nodeName", "ИмяУзла"},
	{"serviceAccountName", "ПодотчётноеЛицо"}, {"tolerations", "Допуски"}, {"affinity", "Привязка"},
	{"livenessProbe", "ПробаЖивости"}, {"readinessProbe", "ПробаГотовности"}, {"startupProbe", "ПробаЗапуска"},
	{"httpGet", "HTTPЗапрос"}, {"path", "Путь"}, {"exec", "Выполнение"}, {"tcpSocket", "TCPСокет"},
	{"initialDelaySeconds", "НачальнаяЗадержкаСекунд"}, {"periodSeconds", "ПериодСекунд"},
	{"schedule", "Расписание"}, {"jobTemplate", "ШаблонЗадания"}, {"backoffLimit", "ЛимитПовторов"},
	{"rules", "Правила"}, {"host", "Хост"}, {"http", "HTTP"}, {"paths", "Пути"}, {"pathType", "ТипПути"},
	{"backend", "Получатель"}, {"service", "Услуга"}, {"number", "Номер"}, {"tls", "TLS"}, {"hosts", "Хосты"},
	{"ingressClassName", "КлассПроходной"}, {"clusterIP", "КластерныйАдрес"},
	{"accessModes", "РежимыДоступа"}, {"storageClassName", "КлассХранения"}, {"storage", "Объём"},
	{"hard", "Жёсткие"}, {"used", "Использовано"}, {"strategy", "Стратегия"},
	{"minReplicas", "МинимумРеплик"}, {"maxReplicas", "МаксимумРеплик"}, {"scaleTargetRef", "ЦельМасштабирования"},
	{"minAvailable", "МинимумДоступных"}, {"maxUnavailable", "МаксимумНедоступных"},
	{"phase", "Фаза"}, {"conditions", "Условия"}, {"reason", "Причина"}, {"message", "Сообщение"},
	{"readyReplicas", "ГотовыхРеплик"}, {"availableReplicas", "ДоступныхРеплик"}, {"podIP", "АдресПода"},
	{"hostIP", "АдресУзла"}, {"startTime", "ВремяЗапуска"}, {"containerStatuses", "СостоянияКонтейнеров"},
	{"restartCount", "ЧислоПерезапусков"}, {"ready", "Готов"}, {"capacity", "Ёмкость"}, {"allocatable", "Доступно"},
	{"progressDeadlineSeconds", "СрокПродвиженияСекунд"}, {"revisionHistoryLimit", "ГлубинаИстории"},
	{"rollingUpdate", "ПостепенноеОбновление"}, {"maxSurge", "МаксимумСверх"}, {"sessionAffinity", "ПривязкаСеанса"},
	{"internalTrafficPolicy", "ПолитикаВнутреннегоТрафика"}, {"externalTrafficPolicy", "ПолитикаВнешнегоТрафика"},
	{"clusterIPs", "КластерныеАдреса"}, {"ipFamilies", "СемействаАдресов"}, {"ipFamilyPolicy", "ПолитикаСемействАдресов"},
	{"dnsPolicy", "ПолитикаDNS"}, {"schedulerName", "ИмяПланировщика"}, {"securityContext", "КонтекстБезопасности"},
	{"terminationGracePeriodSeconds", "СрокЗавершенияСекунд"}, {"terminationMessagePath", "ПутьСообщенияЗавершения"},
	{"terminationMessagePolicy", "ПолитикаСообщенияЗавершения"}, {"observedGeneration", "НаблюдаемоеПоколение"},
	{"updatedReplicas", "ОбновлённыхРеплик"}, {"unavailableReplicas", "НедоступныхРеплик"},
	{"lastTransitionTime", "ВремяПерехода"}, {"lastUpdateTime", "ВремяОбновления"}, {"qosClass", "КлассОбслуживания"},
	{"containerID", "ИдентификаторКонтейнера"}, {"imageID", "ИдентификаторОбраза"}, {"started", "Запущен"},
	{"state", "Положение"}, {"lastState", "ПрошлоеПоложение"}, {"running", "Работает"}, {"terminated", "Завершён"},
	{"waiting", "Ожидает"}, {"startedAt", "ЗапущенВ"}, {"finishedAt", "ЗавершёнВ"}, {"exitCode", "КодВыхода"},
	{"hostname", "ИмяХоста"}, {"podIPs", "АдресаПода"}, {"hostIPs", "АдресаУзла"}, {"ip", "Адрес"},
	{"enableServiceLinks", "ВключитьСсылкиНаУслуги"}, {"preemptionPolicy", "ПолитикаВытеснения"},
	{"priority", "Приоритет"}, {"priorityClassName", "КлассПриоритета"}, {"successThreshold", "ПорогУспеха"},
	{"failureThreshold", "ПорогНеудач"}, {"timeoutSeconds", "ТаймаутСекунд"}, {"scheme", "Схема"},
	{"loadBalancer", "Балансировщик"}, {"ingress", "Входящий"}, {"nodeInfo", "СведенияОбУзле"},
	{"addresses", "Адреса"}, {"address", "ЗначениеАдреса"}, {"kubeletVersion", "ВерсияKubelet"},
	{"volumeClaimTemplates", "ШаблоныЗаявок"}, {"serviceName", "ИмяУслуги"}, {"podManagementPolicy", "ПолитикаЗапускаПодов"},
	{"fsGroup", "ГруппаФайлов"}, {"runAsUser", "ПользовательЗапуска"}, {"runAsGroup", "ГруппаЗапуска"},
	{"metrics", "Показатели"}, {"averageUtilization", "СредняяЗагрузка"},
	{"verbs", "Действия"}, {"apiGroups", "ГруппыAPI"}, {"apiGroup", "ГруппаAPI"}, {"subjects", "Субъекты"}, {"roleRef", "СсылкаНаРоль"},
}

// Fields whose keys are data, not schema: never translated.
var freeFormFields = map[string]bool{
	"labels": true, "annotations": true, "data": true, "stringData": true, "binaryData": true,
	"matchLabels": true, "nodeSelector": true, "parameters": true, "hard": true, "used": true,
}

var (
	ruToEn   = map[string]string{}
	enToRu   = map[string]string{}
	kindByRu = map[string]*Kind{}
	kindByEn = map[string]*Kind{}
)

func fold(s string) string { return strings.ReplaceAll(strings.ToLower(s), "ё", "е") }

func init() {
	for _, p := range fieldPairs {
		ruToEn[fold(p[1])] = p[0]
		if _, ok := enToRu[p[0]]; !ok {
			enToRu[p[0]] = p[1]
		}
	}
	for i := range Kinds {
		k := &Kinds[i]
		kindByEn[strings.ToLower(k.Kind)] = k
		kindByEn[k.Resource] = k
		for _, n := range append([]string{k.Ru, k.Plural}, k.Aliases...) {
			kindByRu[fold(n)] = k
			sk := skeleton(fold(n))
			if prev, seen := kindBySkeleton[sk]; seen && prev != k {
				kindBySkeleton[sk] = nil
			} else {
				kindBySkeleton[sk] = k
			}
		}
	}
}

// LookupKind finds a kind by any of its names: Russian, plural, alias,
// English kind or API resource.
func LookupKind(name string) (*Kind, bool) {
	if k, ok := kindByRu[fold(name)]; ok {
		return k, true
	}
	if k, ok := kindByEn[strings.ToLower(name)]; ok {
		return k, true
	}
	// «получить услугу», «выгрузить развёртыванием»: the word in any case
	st := stem(fold(name))
	for n, k := range kindByRu {
		if stem(n) == st {
			return k, true
		}
	}
	// «информационнуюбазу»: every word declined; compare the consonants
	if k, ok := kindBySkeleton[skeleton(fold(name))]; ok && k != nil {
		return k, true
	}
	return nil, false
}

// kindBySkeleton maps consonant skeletons to kinds; nil marks a skeleton
// two kinds share, which then matches neither.
var kindBySkeleton = map[string]*Kind{}

func skeleton(s string) string {
	var b strings.Builder
	for _, r := range s {
		if !strings.ContainsRune("аеёиоуыэюяйьъ", r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var endings = []string{"ами", "ями", "ого", "его", "ому", "ему", "ыми", "ими", "ой", "ей", "ом", "ем",
	"ов", "ев", "ам", "ям", "ах", "ях", "ую", "юю", "ая", "яя", "ое", "ее", "ые", "ие", "ый", "ий",
	"у", "ю", "а", "я", "ы", "и", "е", "о", "ь"}

// stem cuts a case ending off the last word of a name, roughly: enough to
// tell «услугу» from «узлы» and to match «услуги» with «услуга».
func stem(s string) string {
	r := []rune(s)
	for _, e := range endings {
		er := []rune(e)
		if len(r) > len(er)+2 && string(r[len(r)-len(er):]) == e {
			return string(r[:len(r)-len(er)])
		}
	}
	return s
}

// FieldToAPI translates one Russian field name; unknown names pass as they
// are, so English can always be written directly.
func FieldToAPI(name string) string {
	if en, ok := ruToEn[fold(name)]; ok {
		return en
	}
	return name
}

// FieldToRu translates one API field name for display.
func FieldToRu(name string) string {
	if ru, ok := enToRu[name]; ok {
		return ru
	}
	return name
}

// GroupToAPI turns a Cyrillic API group into its punycode, which the API
// accepts: «оркестратор.бух/v1» → «xn--...xn--90a0bg/v1».
func GroupToAPI(apiVersion string) string {
	group, version, ok := strings.Cut(apiVersion, "/")
	if !ok {
		return apiVersion
	}
	return punyEncode(group) + "/" + version
}

// GroupToRu turns a punycode group back into Cyrillic.
func GroupToRu(apiVersion string) string {
	group, version, ok := strings.Cut(apiVersion, "/")
	if !ok {
		return apiVersion
	}
	return punyDecode(group) + "/" + version
}
