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
}

var Kinds = []Kind{
	{Kind: "Pod", APIVersion: "v1", Ru: "Под", Plural: "Поды", Resource: "pods", Namespaced: true},
	{Kind: "Deployment", APIVersion: "apps/v1", Ru: "Развёртывание", Plural: "Развёртывания", Aliases: []string{"Развертывание", "Развертывания"}, Resource: "deployments", Namespaced: true},
	{Kind: "StatefulSet", APIVersion: "apps/v1", Ru: "НаборСоСостоянием", Plural: "НаборыСоСостоянием", Resource: "statefulsets", Namespaced: true},
	{Kind: "DaemonSet", APIVersion: "apps/v1", Ru: "НаборНаКаждомУзле", Plural: "НаборыНаКаждомУзле", Resource: "daemonsets", Namespaced: true},
	{Kind: "ReplicaSet", APIVersion: "apps/v1", Ru: "НаборРеплик", Plural: "НаборыРеплик", Resource: "replicasets", Namespaced: true},
	{Kind: "Job", APIVersion: "batch/v1", Ru: "Задание", Plural: "Задания", Resource: "jobs", Namespaced: true},
	{Kind: "CronJob", APIVersion: "batch/v1", Ru: "РегламентноеЗадание", Plural: "РегламентныеЗадания", Resource: "cronjobs", Namespaced: true},
	{Kind: "Service", APIVersion: "v1", Ru: "Услуга", Plural: "Услуги", Aliases: []string{"Сервис", "Сервисы"}, Resource: "services", Namespaced: true},
	{Kind: "Ingress", APIVersion: "networking.k8s.io/v1", Ru: "Проходная", Plural: "Проходные", Aliases: []string{"Вход", "Входы"}, Resource: "ingresses", Namespaced: true},
	{Kind: "NetworkPolicy", APIVersion: "networking.k8s.io/v1", Ru: "СетеваяПолитика", Plural: "СетевыеПолитики", Resource: "networkpolicies", Namespaced: true},
	{Kind: "ConfigMap", APIVersion: "v1", Ru: "Справочник", Plural: "Справочники", Resource: "configmaps", Namespaced: true},
	{Kind: "Secret", APIVersion: "v1", Ru: "Секрет", Plural: "Секреты", Aliases: []string{"БланкСтрогойОтчётности"}, Resource: "secrets", Namespaced: true},
	{Kind: "Namespace", APIVersion: "v1", Ru: "Подразделение", Plural: "Подразделения", Aliases: []string{"ОбластьИмён", "ОбластиИмён"}, Resource: "namespaces"},
	{Kind: "Node", APIVersion: "v1", Ru: "Узел", Plural: "Узлы", Aliases: []string{"Склад", "Склады"}, Resource: "nodes"},
	{Kind: "PersistentVolumeClaim", APIVersion: "v1", Ru: "ЗаявкаНаТом", Plural: "ЗаявкиНаТом", Resource: "persistentvolumeclaims", Namespaced: true},
	{Kind: "PersistentVolume", APIVersion: "v1", Ru: "Том", Plural: "Тома", Resource: "persistentvolumes"},
	{Kind: "StorageClass", APIVersion: "storage.k8s.io/v1", Ru: "КлассХранения", Plural: "КлассыХранения", Resource: "storageclasses"},
	{Kind: "ServiceAccount", APIVersion: "v1", Ru: "ПодотчётноеЛицо", Plural: "ПодотчётныеЛица", Aliases: []string{"ПодотчетноеЛицо", "ПодотчетныеЛица"}, Resource: "serviceaccounts", Namespaced: true},
	{Kind: "Role", APIVersion: "rbac.authorization.k8s.io/v1", Ru: "Роль", Plural: "Роли", Resource: "roles", Namespaced: true},
	{Kind: "ClusterRole", APIVersion: "rbac.authorization.k8s.io/v1", Ru: "РольКластера", Plural: "РолиКластера", Resource: "clusterroles"},
	{Kind: "RoleBinding", APIVersion: "rbac.authorization.k8s.io/v1", Ru: "НазначениеРоли", Plural: "НазначенияРолей", Resource: "rolebindings", Namespaced: true},
	{Kind: "ClusterRoleBinding", APIVersion: "rbac.authorization.k8s.io/v1", Ru: "НазначениеРолиКластера", Plural: "НазначенияРолейКластера", Resource: "clusterrolebindings"},
	{Kind: "ResourceQuota", APIVersion: "v1", Ru: "Лимит", Plural: "Лимиты", Resource: "resourcequotas", Namespaced: true},
	{Kind: "LimitRange", APIVersion: "v1", Ru: "НормыРасхода", Plural: "НормыРасходов", Resource: "limitranges", Namespaced: true},
	{Kind: "HorizontalPodAutoscaler", APIVersion: "autoscaling/v2", Ru: "СезонныйНайм", Plural: "СезонныеНаймы", Resource: "horizontalpodautoscalers", Namespaced: true},
	{Kind: "PodDisruptionBudget", APIVersion: "policy/v1", Ru: "ГрафикОтпусков", Plural: "ГрафикиОтпусков", Resource: "poddisruptionbudgets", Namespaced: true},
	{Kind: "Event", APIVersion: "v1", Ru: "Событие", Plural: "События", Resource: "events", Namespaced: true},
	{Kind: "CustomResourceDefinition", APIVersion: "apiextensions.k8s.io/v1", Ru: "РасширениеКонфигурации", Plural: "РасширенияКонфигурации", Resource: "customresourcedefinitions"},

	// Kinds of the distribution.
	{Kind: "Platform", APIVersion: Group + "/v1", Ru: "Поставка", Plural: "Поставки", Resource: "platforms", Native: true},
	{Kind: "ServerCluster", APIVersion: Group + "/v1", Ru: "КластерСерверов", Plural: "КластерыСерверов", Resource: "serverclusters", Namespaced: true, Native: true},
	{Kind: "Infobase", APIVersion: Group + "/v1", Ru: "ИнформационнаяБаза", Plural: "ИнформационныеБазы", Aliases: []string{"ИБ"}, Resource: "infobases", Namespaced: true, Native: true},
	{Kind: "Publication", APIVersion: Group + "/v1", Ru: "Публикация", Plural: "Публикации", Resource: "publications", Namespaced: true, Native: true},
	{Kind: "Standalone", APIVersion: Group + "/v1", Ru: "АвтономныйСервер", Plural: "АвтономныеСерверы", Resource: "standalones", Namespaced: true, Native: true},
	{Kind: "InfobaseBackup", APIVersion: Group + "/v1", Ru: "Выгрузка", Plural: "Выгрузки", Resource: "infobasebackups", Namespaced: true, Native: true},
	{Kind: "ConfigurationUpdate", APIVersion: Group + "/v1", Ru: "ОбновлениеКонфигурации", Plural: "ОбновленияКонфигурации", Resource: "configurationupdates", Namespaced: true, Native: true},
	{Kind: "ExchangeLink", APIVersion: Group + "/v1", Ru: "ПланОбмена", Plural: "ПланыОбмена", Resource: "exchangelinks", Namespaced: true, Native: true},
	{Kind: "BitrixSite", APIVersion: Group + "/v1", Ru: "Сайт", Plural: "Сайты", Aliases: []string{"СайтБитрикс"}, Resource: "bitrixsites", Namespaced: true, Native: true},
	{Kind: "ProductionCalendar", APIVersion: Group + "/v1", Ru: "ПроизводственныйКалендарь", Plural: "ПроизводственныеКалендари", Resource: "productioncalendars", Native: true},
	{Kind: "CurrencyRates", APIVersion: Group + "/v1", Ru: "КурсыВалют", Plural: "КурсыВалют", Resource: "currencyrates", Native: true},
	{Kind: "ReconciliationAct", APIVersion: Group + "/v1", Ru: "АктСверки", Plural: "АктыСверки", Resource: "reconciliationacts", Native: true},
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
	{"verbs", "Действия"}, {"apiGroups", "ГруппыAPI"}, {"subjects", "Субъекты"}, {"roleRef", "СсылкаНаРоль"},
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
		}
	}
}

// LookupKind finds a kind by any of its names: Russian, plural, alias,
// English kind or API resource.
func LookupKind(name string) (*Kind, bool) {
	if k, ok := kindByRu[fold(name)]; ok {
		return k, true
	}
	k, ok := kindByEn[strings.ToLower(name)]
	return k, ok
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
