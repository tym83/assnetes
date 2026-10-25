# Assnetes (1Ж:Оркестратор 8)

[Русская версия](README.ru.md)

Assnetes is a Kubernetes distribution for accounting departments. Everything people write and read is in Russian, in the language of 1C:Enterprise, the platform Russian accounting runs on:

- **manifests are modules in the 1C language** instead of YAML;
- **operators are written in the 1C language too**, with handlers named after the events of a 1C object module;
- **`1сctl`**, the command line, reads and answers in Russian, and talks like the accounting department;
- a package manager installs 1C:Enterprise, 1C:Accounting, 1C:Trade Management, 1C:CRM and 1C-Bitrix into the cluster, with operators that run their whole lifecycle.

It is a joke that works: the API server, etcd and every controller are real, and so is what they do.

> **Status:** early. The language, the manifests, `1сctl` and the runtime of operators work and run five operators on a test cluster. The operators of the 1C products are being written. See [the design](docs/design.md).

> Assnetes is an unofficial parody. It is not made, endorsed or supported by 1C Company. «1С», «1С:Предприятие» and the names of 1C products are trademarks of 1C Company; Assnetes ships none of its software: you bring your own distribution of the platform and its license.

## A manifest

```bsl
#Область Витрина

Развёртывание = Новый Развёртывание("Витрина", "Ромашка");
Развёртывание.Метаданные.Метки.Вставить("приложение", "витрина");
Развёртывание.Спецификация.Реплики = 2;
Развёртывание.Спецификация.Отбор.ПоМеткам.Вставить("приложение", "витрина");
Развёртывание.Спецификация.Шаблон.Метаданные.Метки.Вставить("приложение", "витрина");

Контейнер = Развёртывание.Спецификация.Шаблон.Спецификация.Контейнеры.Добавить();
Контейнер.Имя = "веб";
Контейнер.Образ = "nginx:1.29-alpine";
Контейнер.Ресурсы.Запросы.Процессор = "50m";

Провести(Развёртывание);

#КонецОбласти
```

```console
$ 1сctl провести --файл витрина.bsl
✓ Развёртывание «Витрина» проведено
  — Провела «Витрина». В следующий раз — со служебкой. (Тамара Петровна)
$ 1сctl получить поды --подразделение Ромашка
ИМЯ                        ГОТОВО   ФАЗА       ПЕРЕЗАПУСКИ   СОСТОЯНИЕ    ВОЗРАСТ
vitrina-74849bf66d-7xft9   1/1      Работает   0             ✓ проведён   23 сек
```

Kinds, fields, names and labels are translated on the way to the API and back: `Развёртывание` is a Deployment, `Спецификация.Реплики` is `spec.replicas`, the name «Витрина» becomes `vitrina` with the original kept in an annotation, so `1сctl выгрузить развёртывание витрина` gives the manifest back in Russian. The kinds of the distribution itself keep their Cyrillic fields in the API: Kubernetes accepts them.

## An operator

An operator is an «обработка»: a module and, if it brings kinds of its own, their description.

```bsl
&Обработка(Вид = "АктСверки")
Процедура ОбработкаПроведения(Объект, Отказ) Экспорт
	// bring the world to the object
КонецПроцедуры

&ПроверкаЗаполнения(Виды = "Развёртывание", Операции = "Создание, Изменение")
Процедура ОбработкаПроверкиЗаполнения(Объект, Отказ, Контекст) Экспорт
	// refuse an object the cluster should not take
КонецПроцедуры

&Расписание("17 * * * *")
Процедура УдалитьПомеченные() Экспорт
	// scheduled work
КонецПроцедуры
```

The runtime, `obrabotchik`, does the rest: watches and queues, server-side apply, owners, finalizers, status, events, admission webhooks under a certificate of its own, schedules and leader election. Modules see the cluster through `Кластер`, with HTTP that checks certificates, an XML reader and the language's JSON.

Five operators come with the distribution, in [obrabotki](obrabotki):

| Operator | What it does |
|---|---|
| [Производственный календарь](obrabotki/proizvodstvennyj-kalendar) | loads the official calendar of working days and refuses rollouts on holidays, weekends and the afternoon before a holiday, unless a «служебная записка» says why |
| [Курсы валют](obrabotki/kursy-valyut) | takes the rates of the Bank of Russia and costs every namespace by its requests, in roubles and other currencies |
| [Дата запрета изменения](obrabotki/data-zapreta) | closes a period: objects created before a namespace's date can be changed only by the chief accountant |
| [Удаление помеченных](obrabotki/udalenie-pomechennyh) | deletes what was marked for deletion, only when nothing refers to it, and says what does otherwise |
| [Акт сверки](obrabotki/akt-sverki) | compares the cluster with manifests in Git and writes up a reconciliation statement |

## Layout

| Path | What |
|---|---|
| `pkg/bsl` | the interpreter of the 1C language |
| `pkg/perevod` | translation between Russian and the API |
| `pkg/manifest` | manifests in the 1C language |
| `pkg/golos` | the voice of the accounting department |
| `pkg/cli`, `cmd/1sctl` | `1сctl` |
| `pkg/obrabotka`, `cmd/obrabotchik` | the runtime of operators |
| `pkg/metadannye` | CRDs from descriptions of kinds |
| `obrabotki` | operators that come with the distribution |

## Build

```bash
go build -o bin/1сctl ./cmd/1sctl
go build -o bin/obrabotchik ./cmd/obrabotchik
go test ./...
```

Licensed under the [Apache License 2.0](LICENSE).
