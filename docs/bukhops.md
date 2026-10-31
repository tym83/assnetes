# BukhOps

[Русская версия](bukhops.ru.md)

GitOps takes the state of a cluster from commits. BukhOps takes it from the books: a journal of business operations, kept the way an accounting department keeps it. Flux applies what the books say, and the books compare the cluster with themselves.

| GitOps | BukhOps |
|---|---|
| a repository | a ledger, «Главная книга» |
| a commit | an operation, «Операция»: entries Dr/Cr and a primary document |
| a file in the repository | a primary document: a manifest in the 1C language whose numbers are balances of accounts |
| a required review | the second signature: the author never signs his own operation |
| `git revert` | сторно: the same entries with a minus; history is never rewritten |
| a protected branch, no force push | operations are never changed or deleted |
| a release freeze | the closed period: operations dated on or before it are refused |
| drift | a discrepancy of сверка: «3 replicas in the cluster, 5 by the books» |
| a source of Flux | a Flux `ExternalArtifact` published by the ledger, «Выписка» |

## How it works

```
operations ──► Главная книга ──► Выписка (ExternalArtifact) ──► Flux Kustomization ──► cluster
     ▲            │  balances, primary documents                                       │
     │            └────────────── сверка: the cluster against the books ◄──────────────┘
 1сctl учёт
```

The operator `glavnaya-kniga`, written in the 1C language like every operator of Assnetes, reconciles each ledger:

1. it takes the operations of the ledger in the order of their dates and decides which the books accept: signed by a second person, a signer of this ledger, dated after the closed period, with both accounts of every entry in the chart of accounts, and a сторно that mirrors an accepted operation not reversed before;
2. it remembers a fingerprint of every accepted operation; if one changes afterwards, past the admission checks, the ledger stops publishing until the document is restored;
3. it adds up the entries into balances of accounts and runs the primary documents of the accepted operations that are not reversed, giving them `Сальдо("01/Витрина")`; the latest document wins for each object;
4. it packs the objects into a tarball that is the same, byte for byte, for the same books, files it in the ConfigMap `<ledger>-vypiska` and announces it as an `ExternalArtifact`; every replica of the operator serves it to Flux on port 9090;
5. it creates a Flux `Kustomization`, «Групповое перепроведение», that applies the statement with pruning;
6. every minute it compares the objects of the statement with the cluster and writes the discrepancies into the status of the ledger.

The admission checks of the same operator refuse a change before it gets into the books: an operation signed in someone else's name, self-approval, a signature by someone who is not a signer of the ledger, a date in the closed period, an edit or a deletion of an operation, a сторно that does not mirror its operation, the deletion of a ledger, and the reopening of a period by someone who does not sign the ledger. The checks are an admission webhook with `failurePolicy: Ignore`, like every check of Assnetes; the ledger enforces the same rules again when it reconciles.

## Requirements

Flux 2.7 or newer with the `ExternalArtifact` feature gate of kustomize-controller on. In Flux 2.9.5 it is still off by default:

```bash
flux install --components=source-controller,kustomize-controller
kubectl -n flux-system patch deploy kustomize-controller --type json \
  -p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--feature-gates=ExternalArtifact=true"}]'
```

The operator comes with the package `assnetes-obrabotki`; without Flux, a ledger keeps its books and says that Flux is missing.

## A ledger

```bsl
Книга = Новый ГлавнаяКнига("Основная", "Бухгалтерия");
Книга.Спецификация.Подразделение = "Ромашка";      // where the documents go
Книга.Спецификация.Подписанты.Добавить("tamara.petrovna");

Счёт = Книга.Спецификация.ПланСчетов.Добавить();
Счёт.Код = "01";
Счёт.Наименование = "Основные средства";

Счёт = Книга.Спецификация.ПланСчетов.Добавить();
Счёт.Код = "80";
Счёт.Наименование = "Уставный капитал";
Счёт.Вид = "Пассивный";

Провести(Книга);
```

## An operation

```bsl
Операция = Новый Операция("БП-000001", "Бухгалтерия");
Операция.Спецификация.Книга = "Основная";
Операция.Спецификация.Дата = "2026-09-01";
Операция.Спецификация.Содержание = "Принята к учёту витрина";

Проводка = Операция.Спецификация.Проводки.Добавить();
Проводка.Дт = "01/Витрина";     // account 01, subaccount «Витрина»
Проводка.Кт = "80";
Проводка.Сумма = 2;

Операция.Спецификация.Первичка =
"Развёртывание = Новый Развёртывание(""Витрина"");
|Развёртывание.Спецификация.Реплики = Сальдо(""01/Витрина"");
|...
|Провести(Развёртывание);";

Провести(Операция);
```

`1сctl провести` fills `Автор` with the user the cluster sees. The full example, with the roles of the bookkeepers and the chief accountant, is in [examples/bukhops](../examples/bukhops).

## 1сctl учёт

```console
$ 1сctl провести --файл bp-000002.bsl                  # an operation, by its author
$ 1сctl --от-имени tamara.petrovna учёт утвердить БП-000002
$ 1сctl учёт операции                                  # the journal: entries, signatures, accepted or why not
$ 1сctl учёт сторно БП-000003                          # a reversing operation, to be signed like any other
$ 1сctl учёт закрыть-период 2026-09-30
$ 1сctl учёт сверка                                    # revision, applied or not, discrepancies
$ 1сctl учёт осв                                       # turnover balance sheet of the open period
```

`--от-имени` acts as another user through impersonation. The balance sheet is a query over the accounting register of the query language:

```console
$ 1сctl запрос 'ВЫБРАТЬ * ИЗ РегистрБухгалтерии.Хозрасчётный.ОстаткиИОбороты КАК О'
```

## Limits

A primary document posts objects whole: two operations that describe the same object do not merge, the later one replaces it. Balances are numbers; whatever is not a number, such as an image, lives in primary documents. The statement is kept in a ConfigMap, so the objects of one ledger must fit into a megabyte packed.
