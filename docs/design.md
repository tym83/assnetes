# Design

[Русская версия](design.ru.md)

## What it is

A Kubernetes distribution in which:

1. manifests are written in the 1C:Enterprise language instead of YAML;
2. operators are written in the same language: a runtime does the machinery of a controller, and the logic lives in `.bsl` modules;
3. a package manager, kubepkg, installs 1C:Enterprise, 1C:Accounting, 1C:Trade Management, 1C:CRM and 1C-Bitrix, and their operators, in the manner of CloudNativePG, run their whole lifecycle: creation, configuration updates, backups, horizontal and vertical scaling;
4. a GUI in the style accountants know manages a cluster and a set of clusters;
5. the CLI, the GUI and the operators talk to people like an accounting department.

The rule is the one of the other fun distributions: a joke outside, real work inside. Grumbling is a layer on top of correct behaviour, never a replacement for it. Scripts and CI get clean output and exit codes.

## Constraints found by research

| Fact | Consequence |
|---|---|
| The distributions of the 1C platform may not be redistributed | No public image carries 1C. Packages ship everything around it; the user brings the platform, and its image is built inside the cluster, into the cluster's own registry |
| Without a support subscription, the platform can be obtained legally from developer.1c.ru under the developer license (a server, 3 sessions, 1 web session) | That is the legal path for a test bench. The free «learning» edition has no server and no web client |
| Templates of 1C:Accounting, Trade Management and CRM come only with a subscription or a purchased box | Packages of applied solutions take the template from a Secret or an OCI artifact |
| 1C-Bitrix and Bitrix24 «box» are public 30-day trials with an official container environment | Bitrix is the one product that installs with nothing brought |
| A 1C server is ragent, rmngr and rphost; clients connect straight to `host:port` of a working process, the host name is in the cluster registry, licenses are bound to hardware | Working servers are a StatefulSet with stable DNS names; the licensing service sits on its own pod pinned to a node with a fixed host name and MAC |
| `ibcmd` creates an infobase, loads `.cf`, `.cfe`, `.dt` and applies a configuration without the Designer; `.cfu` updates need the Designer | Most of the lifecycle runs `ibcmd` and `rac` in Jobs; vendor updates run the Designer under Xvfb |
| 1C needs PostgreSQL with its patch; Postgres Professional publishes a free «PostgreSQL for 1C»; CloudNativePG accepts any image with the right binaries | The database is CloudNativePG with an image of PostgreSQL for 1C |
| No Kubernetes operator for 1C exists | We write the first |
| No interpreter of the 1C language exists in Go; OneScript's HTTPS client does not verify certificates | The interpreter is our own, `pkg/bsl` |
| Kubernetes accepts Cyrillic property names in CRD schemas, but not in kinds, object names, labels or API groups; CEL does not parse Cyrillic | Our kinds keep Cyrillic fields as stored; kinds, names, labels and groups are translated |

## Layers

```
GUI                    1сctl                  voice (personas, calendar)
──────────────────────────────────────────────────────────────────────
the 1C language in Go: manifests · cluster queries · runtime of operators
──────────────────────────────────────────────────────────────────────
operators in the 1C language: platform, Accounting, Trade, CRM, Bitrix,
and six general-purpose ones
──────────────────────────────────────────────────────────────────────
kubepkg, embedded under its own API group, and the «Прайс-лист» repository
──────────────────────────────────────────────────────────────────────
any Kubernetes ≥ 1.33         or         the kuberoot distribution «assnetes»
```

## The language

`pkg/bsl` is a lexer, a recursive-descent parser and an evaluator of the subset of the language that means something outside the platform: all keywords in Russian and English, the preprocessor (`#Если Сервер`, regions, `#Использовать`), annotations, decimal numbers (`0.1 + 0.2 = 0.3`), strings with `|` continuation, dates, the universal collections (Структура, Соответствие, Массив, ТаблицаЗначений with `Свернуть` and `Сортировать`, their fixed variants), strings and dates functions, `Формат`, JSON, errors worded as the platform words them. Execution is bounded by a number of statements and a context; `Выполнить` and `Вычислить` are refused.

## Translation

`pkg/perevod` sits between people and the API:

- kinds and fields go through a dictionary (`Развёртывание` ↔ `Deployment`, `Спецификация.Реплики` ↔ `spec.replicas`), kinds are found in any grammatical case («получить услугу»), and words agree with their gender («Услуга проведена»);
- names, label keys and values and data keys are transliterated, the same name always the same way; what transliteration loses is kept in annotations of the object and restored on reading;
- the API group of the distribution is «оркестратор.бух», in punycode in the API;
- the kinds of the distribution are not translated below `spec` and `status`: their fields are Cyrillic in etcd.

## Manifests

A manifest is a module. `Новый Развёртывание("Имя", "Подразделение")` makes an object whose nested objects and lists appear as they are written; `Провести(...)` posts it; `Параметры` carries `--параметр` values. `1сctl провести` applies with server-side apply. `1сctl выгрузить` turns a live object back into a manifest.

## Operators

An operator is an «обработка»: a directory with a module and, for kinds of its own, `Метаданные.yaml`, a description of реквизиты with types from which `pkg/metadannye` makes a CRD, with Cyrillic printer columns.

| Handler | Role |
|---|---|
| `ОбработкаПроведения(Объект, Отказ)` with `&Обработка(Вид, Владеет, Наблюдает)` | reconcile |
| `ПередУдалением(Объект, Отказ)` | under a finalizer |
| `ОбработкаПроверкиЗаполнения(Объект, Отказ, Контекст)` with `&ПроверкаЗаполнения(Виды, Операции)` | validating admission |
| procedures with `&Расписание("cron")` | scheduled work, on the leader |

`obrabotchik` runs operators with controller-runtime: one interpreter per operator, one call at a time; status written back when the module changed it; a refusal (`Отказ = Истина`) becomes a warning event with what the module said and a retry; webhooks are served under a self-signed certificate registered with `failurePolicy: Ignore`, so a broken operator never stops the cluster. Modules get `Кластер` (Получить, Выбрать, Провести with an owner, Удалить, ПометитьНаУдаление, ЗаписатьСостояние, ЗаписатьСобытие, ВыполнитьВПоде, ПрочитатьСекрет), `Журнал`, `HTTPСоединение`, `ЧтениеXML`, `ДанныеСовпадают`, `РазобратьКоличество`, `ВыполнитьМанифест`, `МестноеВремя`.

The six general-purpose operators are in the [README](../README.md).

## Packages and the operators of the products

kubepkg is embedded through its Go libraries under the group «пакеты.бух». The repository is «Прайс-лист»: `1сctl закупить бухгалтерия`, `1сctl прайс`, `1сctl остатки`, `1сctl сторно`.

| Package | Installs | Binaries |
|---|---|---|
| `postgresql-1c` | CloudNativePG and an image of PostgreSQL for 1C | built from the free packages of Postgres Professional |
| `1c-predpriyatie` | the operator of the platform | brought by the user, built in the cluster |
| `1c-buhgalteriya` | 1C:Accounting | template brought by the user |
| `1c-upravlenie-torgovlej` | 1C:Trade Management | template brought by the user |
| `1c-crm` | 1C:CRM | template brought by the user |
| `1c-bitrix`, `bitrix24` | 1C-Bitrix site management, Bitrix24 box | public trials |
| `assnetes` | the six operators, the GUI, the voice | ours |

Without a platform, a package still installs, and its infobases wait with a status that says what is missing and where to get it.

### The operator of the platform

| Kind | Russian | What |
|---|---|---|
| `Platform` | Поставка | a version of the platform, where its archive is, the image built from it |
| `ServerCluster` | КластерСерверов | central server, working servers, licensing, parameters of the cluster |
| `Infobase` | ИнформационнаяБаза | an infobase on a cluster: DBMS, configuration, extensions, users, backups |
| `Publication` | Публикация | Apache with wsap24 and `default.vrd`: web client, HTTP services, OData |
| `Standalone` | АвтономныйСервер | `ibsrv` with a file infobase, one replica |
| `InfobaseBackup` | Выгрузка | a backup: `.dt` or a physical one by CloudNativePG |
| `ConfigurationUpdate` | ОбновлениеКонфигурации | an update of `.cf`, `.cfu`, `.cfe` in a window of work |

**Horizontal scaling** adds a working server: a pod, `rac server insert`, `rac rule apply`; scaling down takes the server out of the rules, waits for sessions to end, telling users, then `rac server remove`. An autoscaler works from sessions and rphost load, bounded by licenses.

**Vertical scaling** resizes pods in place and sets `--safe-working-processes-memory-limit`, `--critical-total-memory`, `--infobases-limit` and `--connections-limit` with `rac server update` to match.

**The lifecycle of an infobase:** creation with `ibcmd infobase create`; updates with sessions locked by a permission code, users told to leave, a backup first, `ibcmd config load/apply` or the Designer, the update handlers run, the lock lifted, the window taken from the production calendar; backups by CloudNativePG and `.dt` on a schedule; testing and repair on a schedule; deletion after a mark and a backup.

Without real 1C at hand, the operator is tested end to end against a stand-in image that answers `rac`, `ibcmd` and `ragent` the way the documentation says they answer.

### Applied solutions and exchanges

Thin operators over `Infobase` know their configuration, the minimal platform of every release, the edition (basic → `Standalone`, PROF/CORP → `ServerCluster`) and resource profiles. `ExchangeLink` («ПланОбмена») connects bases: EnterpriseData between Trade and Accounting, CommerceML between Trade and Bitrix, distributed infobases between clusters.

### Bitrix

`BitrixSite` («Сайт»): nginx and php-fpm with an autoscaler, exactly one cron for agents, the push server with Redis, sessions and cache in Redis, `upload/` on a shared volume or S3, MariaDB or PostgreSQL, the core on a shared volume updated by one pod at a time, the license key in a Secret and the trial period watched.

## The voice

Phrases live in `pkg/golos/frazy.yaml`, by situation and persona: Тамара Петровна the chief accountant, Леночка the junior, Валентина Ивановна the cashier, the girls of the department, the tax inspector, the auditor, the director, Димон the 1C man. API errors are worded the way the 1C platform words them, with the original underneath. The accounting calendar counts: the 25th, the 28th, the closing of a month, the 21st of November. Lunch is from 12 to 13. Nothing of this reaches pipes, CI or non-table output.

## The GUI

A web application in the manner of 1C's «Taxi» interface, with a palette of its own: the list of infobases is the list of clusters; sections; lists of objects with the icons of posted, written and marked documents; object forms with «Провести и закрыть»; «Движения документа» show what an object created; reports (a turnover balance sheet of resources, costs by namespace, an express check of the cluster); processings (closing of the month, deletion of marked objects, testing and repair, reconciliation); the event log; a query console.

## More from the world of 1C

A HASP key as a device plugin; a support subscription as an entitlement that gates updates; «Честный знак» as signature verification of images; a frozen bank account as a deny-egress policy; «кофта на стуле» as a liveness probe; «Сколько надо?» as rightsizing; «Ил-2 Штурмовик» as chaos testing.

## Plan

| Phase | What | State |
|---|---|---|
| 0 | test cluster in a Cozystack tenant, CI | done |
| 1 | the interpreter | done |
| 2 | manifests, translation, `1сctl`, the voice | done |
| 3 | the runtime of operators and five operators | done |
| 4 | PostgreSQL for 1C, the operator of Bitrix, packages | |
| 5 | the operator of the platform, against the stand-in | |
| 6 | Accounting, Trade, CRM, exchanges | |
| 7 | queries over the cluster and accumulation registers | |
| 8 | the GUI | |
| 9 | the kuberoot profile, a release, an article | |
