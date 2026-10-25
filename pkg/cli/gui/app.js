"use strict";

// 1Ж:Предприятие — окно для кластеров, в привычном бухгалтеру виде.

const sost = { baza: null, vkladki: [], aktivnaya: null };
const $ = (id) => document.getElementById(id);
const el = (tag, attrs = {}, ...deti) => {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "onclick" || k === "ondblclick" || k === "oninput" || k === "onchange") e[k] = v;
    else if (k === "class") e.className = v;
    else e.setAttribute(k, v);
  }
  for (const d of deti.flat()) if (d !== null && d !== undefined) e.append(d.nodeType ? d : document.createTextNode(String(d)));
  return e;
};

async function api(put, telo) {
  const opts = telo === undefined ? {} : { method: "POST", body: telo };
  const r = await fetch(put, opts);
  const dannye = await r.json().catch(() => ({ ошибка: "Ошибка при выполнении операции с информационной базой.", подробнее: r.statusText }));
  if (!r.ok) {
    oshibka(dannye.ошибка, dannye.подробнее);
    throw new Error(dannye.подробнее || dannye.ошибка);
  }
  return dannye;
}

function oshibka(tekst, podrobnee) {
  $("tekst-oshibki").textContent = tekst || "Ошибка";
  $("podrobnee").textContent = podrobnee || "";
  $("zatemnenie").hidden = false;
}
$("ok").onclick = () => { $("zatemnenie").hidden = true; };

function uvedomlenie(tekst, kto) {
  const u = el("div", { class: "uvedomlenie" }, tekst, kto ? el("div", { class: "kto" }, "— " + kto) : null);
  document.body.append(u);
  setTimeout(() => u.remove(), 6000);
}

// ——— Запуск ———

async function zapusk() {
  const bazy = await api("/api/bazy");
  const spisok = $("bazy");
  spisok.innerHTML = "";
  if (!bazy.length) spisok.append(el("div", { class: "pusto" }, "Список информационных баз пуст. Добавьте кластер в kubeconfig."));
  for (const b of bazy) {
    const stroka = el("div", { class: "baza" + (b.Текущая ? " vybrana" : "") }, b.Имя, el("small", {}, b.Сервер));
    stroka.onclick = () => {
      spisok.querySelectorAll(".baza").forEach((x) => x.classList.remove("vybrana"));
      stroka.classList.add("vybrana");
      sost.baza = b.Имя;
    };
    stroka.ondblclick = () => { sost.baza = b.Имя; otkryt(); };
    if (b.Текущая) sost.baza = b.Имя;
    spisok.append(stroka);
  }
}
$("otkryt").onclick = () => otkryt();
$("konfigurator").onclick = () => { otkryt(); konsol(); };
$("vyhod").onclick = () => { document.body.innerHTML = "<div class='pusto'>Работа завершена. Не забудьте сдать отчётность до 25-го.</div>"; };

async function otkryt() {
  if (!sost.baza) { oshibka("Выберите информационную базу.", ""); return; }
  $("zapusk").hidden = true;
  $("prilozhenie").hidden = false;
  $("nazvanie-bazy").textContent = "Кластер «" + sost.baza + "»";
  document.title = sost.baza + " / 1Ж:Предприятие";
  razdely();
  vybratRazdel(RAZDELY[0]);
  golos();
  setInterval(golos, 45000);
}

// ——— Разделы ———

const RAZDELY = [
  { ima: "Главное", ssylki: [["Монитор руководителя", () => monitor()], ["Подразделения", () => spisok("Подразделения")]] },
  { ima: "Персонал", gruppy: { "Рабочие места": ["Поды", "Развёртывания", "НаборыСоСостоянием", "НаборыНаКаждомУзле"], "Разовые работы": ["Задания", "РегламентныеЗадания"], "Кадры": ["ПодотчётныеЛица", "СезонныеНаймы", "ГрафикиОтпусков"] } },
  { ima: "Склад", gruppy: { "Основные средства": ["Узлы"], "Материалы": ["Тома", "ЗаявкиНаТом", "КлассыХранения"] } },
  { ima: "Продажи", gruppy: { "Покупатели": ["Услуги", "Проходные", "СетевыеПолитики"] } },
  { ima: "Справочники", gruppy: { "Справочники": ["Справочники", "Секреты", "Подразделения"], "Лимиты": ["Лимиты", "НормыРасхода"] } },
  { ima: "1С:Предприятие", gruppy: { "Кластеры": ["КластерыСерверов", "Поставки"], "Базы": ["ИнформационныеБазы", "Публикации", "ШаблоныКонфигураций"], "Сервис": ["ОбновленияКонфигурации", "Выгрузки"], "Битрикс": ["Сайты"] } },
  { ima: "Бухгалтерия", gruppy: { "Учёт": ["ПроизводственныеКалендари", "КурсыВалют", "АктыСверки"] } },
  { ima: "Отчеты", ssylki: [["Оборотно-сальдовая ведомость", () => osv()], ["Перезапуски за период", () => perezapuski()], ["Консоль запросов", () => konsol()]] },
  { ima: "Администрирование", ssylki: [["Журнал регистрации", () => zhurnal()], ["Удаление помеченных объектов", () => pomechennye()], ["Расширения конфигурации", () => spisok("РасширенияКонфигурации")]] },
];

function razdely() {
  const nav = $("razdely");
  nav.innerHTML = "";
  for (const r of RAZDELY) {
    r.uzel = el("div", { class: "razdel", onclick: () => vybratRazdel(r) }, r.ima);
    nav.append(r.uzel);
  }
}

function vybratRazdel(r) {
  RAZDELY.forEach((x) => x.uzel.classList.toggle("vybran", x === r));
  const nav = $("navigaciya");
  nav.innerHTML = "";
  if (r.ssylki) for (const [ima, f] of r.ssylki) nav.append(el("a", { onclick: f }, ima));
  if (r.gruppy) for (const [gruppa, vidy] of Object.entries(r.gruppy)) {
    nav.append(el("h4", {}, gruppa));
    for (const v of vidy) nav.append(el("a", { onclick: () => spisok(v) }, raskryt(v)));
  }
}

// «ЗаявкиНаТом» → «Заявки на том»
function raskryt(s) {
  return s.replace(/([а-яё])([А-ЯЁ])/g, "$1 $2").replace(/ ([А-ЯЁ])([а-яё])/g, (m, a, b) => " " + a.toLowerCase() + b);
}

// ——— Вкладки ———

function vkladka(kluch, zagolovok, risovat) {
  let v = sost.vkladki.find((x) => x.kluch === kluch);
  if (!v) {
    v = { kluch, zagolovok, risovat };
    sost.vkladki.push(v);
  }
  v.risovat = risovat;
  sost.aktivnaya = v;
  vkladki();
  v.risovat($("forma"));
}

function vkladki() {
  const p = $("vkladki");
  p.innerHTML = "";
  for (const v of sost.vkladki) {
    const z = el("span", { class: "zakryt", onclick: (e) => { e.stopPropagation(); zakryt(v); } }, "×");
    p.append(el("div", { class: "vkladka" + (v === sost.aktivnaya ? " aktivna" : ""), onclick: () => { sost.aktivnaya = v; vkladki(); v.risovat($("forma")); } }, v.zagolovok, z));
  }
}

function zakryt(v) {
  sost.vkladki = sost.vkladki.filter((x) => x !== v);
  if (sost.aktivnaya === v) sost.aktivnaya = sost.vkladki[sost.vkladki.length - 1] || null;
  vkladki();
  if (sost.aktivnaya) sost.aktivnaya.risovat($("forma"));
  else $("forma").innerHTML = "";
}

const b = (s) => encodeURIComponent(s);

// ——— Форма списка ———

function znachok(sostoyanie, pometka) {
  if (pometka || /помечен/.test(sostoyanie)) return el("span", { class: "pomechen", title: "Помечен на удаление" }, "✗");
  if (/проведён/.test(sostoyanie)) return el("span", { class: "proveden", title: "Проведён" }, "✓");
  if (/ошибка|закрыт/.test(sostoyanie)) return el("span", { class: "oshibka-provedeniya", title: sostoyanie }, "!");
  return el("span", { class: "zapisan", title: "Записан, не проведён" }, "○");
}

function spisok(vid) {
  const filtr = { podrazdelenie: "" };
  vkladka("spisok:" + vid, raskryt(vid), async (forma) => {
    forma.innerHTML = "";
    const zag = el("h1", {}, raskryt(vid));
    const pole = el("input", { placeholder: "Подразделение: все", value: filtr.podrazdelenie });
    const panel = el("div", { class: "panel-knopok" },
      el("button", { class: "osn", onclick: () => sozdat(vid) }, "Создать"),
      el("button", { onclick: () => pometit(vid, vybrannaya, false) }, "Пометить на удаление"),
      el("button", { onclick: () => pometit(vid, vybrannaya, true) }, "Снять пометку"),
      el("button", { onclick: () => sost.aktivnaya.risovat(forma) }, "Обновить"),
      el("span", { class: "razdelitel" }), pole);
    let vybrannaya = null;
    pole.onchange = () => { filtr.podrazdelenie = pole.value; sost.aktivnaya.risovat(forma); };
    forma.append(zag, panel);
    let d;
    try { d = await api(`/api/${b(sost.baza)}/spisok/${b(vid)}?podrazdelenie=${b(filtr.podrazdelenie)}`); } catch { return; }
    zag.textContent = raskryt(d.Заголовок);
    const tablica = el("table", { class: "spisok" });
    const sostCol = d.Колонки.indexOf("СОСТОЯНИЕ");
    tablica.append(el("tr", {}, el("th", { class: "znachok" }, ""), d.Колонки.map((k) => el("th", {}, k))));
    for (const s of d.Строки || []) {
      const tr = el("tr", {}, el("td", { class: "znachok" }, znachok(s.Ячейки[sostCol] || "", s.Пометка)), s.Ячейки.map((c) => el("td", {}, c)));
      tr.onclick = () => { tablica.querySelectorAll("tr").forEach((x) => x.classList.remove("vybrana")); tr.classList.add("vybrana"); vybrannaya = s; };
      tr.ondblclick = () => obekt(vid, s.Подразделение || "-", s.ИмяAPI || s.Имя, s.Имя);
      tablica.append(tr);
    }
    if (!(d.Строки || []).length) forma.append(el("div", { class: "pusto" }, "Остатков нет. И не было."));
    else forma.append(tablica);
  });
}

async function pometit(vid, s, snyat) {
  if (!s) { oshibka("Выберите строку списка.", ""); return; }
  const d = await api(`/api/${b(sost.baza)}/pometit/${b(vid)}/${b(s.Подразделение || "-")}/${b(s.ИмяAPI || s.Имя)}${snyat ? "?snyat=1" : ""}`, "");
  uvedomlenie(snyat ? `Пометка на удаление снята: «${s.Имя}»` : (d.Фраза || `«${s.Имя}» помечен на удаление`), snyat ? "" : "Тамара Петровна");
  sost.aktivnaya.risovat($("forma"));
}

// ——— Форма объекта ———

function obekt(vid, ns, imaAPI, ima) {
  vkladka(`obekt:${vid}:${ns}:${imaAPI}`, `${raskryt(vid).replace(/ы$|и$/, "")}: ${ima}`, async (forma) => {
    forma.innerHTML = "";
    let d;
    try { d = await api(`/api/${b(sost.baza)}/obekt/${b(vid)}/${b(ns)}/${b(imaAPI)}`); } catch { return; }
    const o = d.Объект;
    const md = o.Метаданные || {};
    forma.append(el("h1", {}, `${o.Вид} ${md.Имя || ima}`, el("span", { class: "zapisan" }, md.ДатаПометкиНаУдаление ? " (помечен на удаление)" : " (проведён)")));
    const kod = el("textarea", { class: "kod" });
    kod.value = d.Манифест;
    const provesti = async (zakryt_) => {
      try {
        const r = await api(`/api/${b(sost.baza)}/provesti`, kod.value);
        uvedomlenie((r.Проведено || []).join("; ") + ". " + (r.Фраза || ""), "Тамара Петровна");
        if (zakryt_) zakryt(sost.aktivnaya); else sost.aktivnaya.risovat(forma);
      } catch {}
    };
    forma.append(el("div", { class: "panel-knopok" },
      el("button", { class: "osn", onclick: () => provesti(true) }, "Провести и закрыть"),
      el("button", { onclick: () => provesti(false) }, "Записать"),
      el("button", { onclick: () => provesti(false) }, "Провести"),
      el("button", { onclick: () => pometit(vid, { Имя: ima, ИмяAPI: imaAPI, Подразделение: ns === "-" ? "" : ns }, false) }, "Пометить на удаление")));
    const rek = el("div", { class: "rekvizity" });
    const dobavit = (k, v) => { if (v !== undefined && v !== null && v !== "") rek.append(el("div", { class: "ima" }, k), el("div", {}, v)); };
    dobavit("Наименование", md.Имя);
    dobavit("Подразделение", md.Подразделение);
    dobavit("Дата создания", md.ДатаСоздания);
    dobavit("Метки", md.Метки ? Object.entries(md.Метки).map(([k, v]) => `${k} = ${v}`).join("; ") : "");
    if (o.Состояние) dobavit("Фаза", o.Состояние.Фаза);
    if (o.Состояние) dobavit("Сообщение", o.Состояние.Сообщение);
    forma.append(rek);
    const zakladki = el("div", { class: "zakladki" });
    const telo = el("div");
    const stranicy = {
      "Как .bsl": () => [kod],
      "Спецификация": () => [el("pre", { class: "derevo" }, derevo(o.Спецификация))],
      "Состояние": () => [el("pre", { class: "derevo" }, derevo(o.Состояние))],
      "Движения документа": () => {
        if (!d.Движения || !d.Движения.length) return [el("div", { class: "pusto" }, "Документ не формирует движений.")];
        const t = el("table", { class: "spisok" }, el("tr", {}, el("th", {}, "Регистр"), el("th", {}, "Запись"), el("th", {}, "Подразделение")));
        for (const m of d.Движения) t.append(el("tr", {}, el("td", {}, m.Вид), el("td", {}, m.Имя), el("td", {}, m.Подразделение)));
        return [t];
      },
    };
    const pokazat = (ima_) => {
      zakladki.querySelectorAll(".zakladka").forEach((z) => z.classList.toggle("aktivna", z.textContent === ima_));
      telo.innerHTML = "";
      telo.append(...stranicy[ima_]());
    };
    for (const ima_ of Object.keys(stranicy)) zakladki.append(el("div", { class: "zakladka", onclick: () => pokazat(ima_) }, ima_));
    forma.append(zakladki, telo);
    pokazat("Как .bsl");
  });
}

function derevo(v, otstup = "") {
  if (v === undefined || v === null) return "—";
  if (typeof v !== "object") return String(v);
  if (Array.isArray(v)) return v.map((x) => `${otstup}- ${typeof x === "object" ? "\n" + derevo(x, otstup + "    ") : x}`).join("\n");
  return Object.entries(v).map(([k, x]) => typeof x === "object" && x !== null ? `${otstup}${k}:\n${derevo(x, otstup + "  ")}` : `${otstup}${k}: ${x}`).join("\n");
}

function sozdat(vid) {
  const shablony = {
    "Развёртывания": `Развёртывание = Новый Развёртывание("Новое", "Подразделение");\nРазвёртывание.Спецификация.Реплики = 1;\nРазвёртывание.Спецификация.Отбор.ПоМеткам.Вставить("приложение", "новое");\nРазвёртывание.Спецификация.Шаблон.Метаданные.Метки.Вставить("приложение", "новое");\nКонтейнер = Развёртывание.Спецификация.Шаблон.Спецификация.Контейнеры.Добавить();\nКонтейнер.Имя = "веб";\nКонтейнер.Образ = "nginx:1.29-alpine";\n\nПровести(Развёртывание);\n`,
  };
  const ima = raskryt(vid);
  vkladka("sozdat:" + vid, ima + " (создание)", (forma) => {
    forma.innerHTML = "";
    const kod = el("textarea", { class: "kod" });
    kod.value = shablony[vid] || `// ${ima}: опишите документ на языке 1С и проведите его.\n\nПровести(Объект);\n`;
    forma.append(el("h1", {}, ima + " (создание)"),
      el("div", { class: "panel-knopok" }, el("button", { class: "osn", onclick: async () => {
        try {
          const r = await api(`/api/${b(sost.baza)}/provesti`, kod.value);
          uvedomlenie((r.Проведено || []).join("; ") + ". " + (r.Фраза || ""), "Тамара Петровна");
          zakryt(sost.aktivnaya);
          spisok(vid);
        } catch {}
      } }, "Провести и закрыть")), kod);
  });
}

// ——— Отчёты и обработки ———

async function tablicaZaprosa(forma, tekst) {
  let d;
  try { d = await api(`/api/${b(sost.baza)}/zapros`, tekst); } catch { return; }
  if (!d.Строки || !d.Строки.length) { forma.append(el("div", { class: "pusto" }, "Результат запроса пуст.")); return; }
  const t = el("table", { class: "spisok" }, el("tr", {}, d.Колонки.map((k) => el("th", {}, raskryt(k)))));
  for (const s of d.Строки) t.append(el("tr", {}, s.map((c) => el("td", {}, c))));
  forma.append(t);
}

function osv() {
  vkladka("osv", "ОСВ", async (forma) => {
    forma.innerHTML = "";
    forma.append(el("div", { class: "otchet-zagolovok" }, "Оборотно-сальдовая ведомость по счёту 01 «Основные средства», субконто «Узлы»"),
      el("div", { class: "otchet-podzagolovok" }, `Кластер «${sost.baza}», на ${new Date().toLocaleString("ru-RU")}. Дебет — что даёт узел, кредит — что запросили поды, сальдо — что осталось. Ядра — в штуках, память — в ГиБ.`));
    await tablicaZaprosa(forma, `ВЫБРАТЬ О.Узел КАК Узел, О.ПроцессорПриход КАК ЯдраОборотДт, О.ПроцессорРасход КАК ЯдраОборотКт, О.ПроцессорОстаток КАК ЯдраСальдоДт,
      О.ПамятьПриход КАК ПамятьОборотДт, О.ПамятьРасход КАК ПамятьОборотКт, О.ПамятьОстаток КАК ПамятьСальдоДт, О.ПодыРасход КАК Подов
      ИЗ РегистрНакопления.РесурсыУзлов.ОстаткиИОбороты КАК О УПОРЯДОЧИТЬ ПО О.Узел`);
  });
}

function perezapuski() {
  vkladka("perezapuski", "Перезапуски", async (forma) => {
    forma.innerHTML = "";
    forma.append(el("div", { class: "otchet-zagolovok" }, "Обороты регистра «Перезапуски»"));
    await tablicaZaprosa(forma, `ВЫБРАТЬ ПЕРВЫЕ 30 О.Подразделение, О.Под, О.Контейнер, О.ПерезапускиОборот КАК Перезапусков
      ИЗ РегистрНакопления.Перезапуски.Обороты КАК О ГДЕ О.ПерезапускиОборот > 0 УПОРЯДОЧИТЬ ПО Перезапусков УБЫВ`);
  });
}

function monitor() {
  vkladka("monitor", "Монитор руководителя", async (forma) => {
    forma.innerHTML = "";
    forma.append(el("h1", {}, "Монитор руководителя"));
    await tablicaZaprosa(forma, `ВЫБРАТЬ П.Подразделение КАК Подразделение, КОЛИЧЕСТВО(*) КАК Подов,
      СУММА(ВЫБОР КОГДА П.Фаза = "Running" ТОГДА 1 ИНАЧЕ 0 КОНЕЦ) КАК Работает,
      СУММА(ВЫБОР КОГДА П.Фаза <> "Running" И П.Фаза <> "Succeeded" ТОГДА 1 ИНАЧЕ 0 КОНЕЦ) КАК ВнеГрафика
      ИЗ Кластер.Поды КАК П СГРУППИРОВАТЬ ПО П.Подразделение УПОРЯДОЧИТЬ ПО Подов УБЫВ`);
  });
}

function konsol() {
  vkladka("konsol", "Консоль запросов", (forma) => {
    forma.innerHTML = "";
    const kod = el("textarea", { class: "kod", style: "min-height:180px" });
    kod.value = `ВЫБРАТЬ\n\tП.Подразделение КАК Подразделение,\n\tКОЛИЧЕСТВО(*) КАК Подов\nИЗ\n\tКластер.Поды КАК П\nГДЕ\n\tП.Фаза = "Running"\nСГРУППИРОВАТЬ ПО\n\tП.Подразделение\nУПОРЯДОЧИТЬ ПО\n\tПодов УБЫВ`;
    const rezultat = el("div");
    forma.append(el("h1", {}, "Консоль запросов"), kod,
      el("div", { class: "panel-knopok", style: "margin-top:8px" }, el("button", { class: "osn", onclick: async () => { rezultat.innerHTML = ""; await tablicaZaprosa(rezultat, kod.value); } }, "Выполнить")),
      rezultat);
  });
}

function zhurnal() {
  const filtr = { vazhnost: "" };
  vkladka("zhurnal", "Журнал регистрации", async (forma) => {
    forma.innerHTML = "";
    const vybor = el("select", {}, ["", "Ошибка", "Информация"].map((v) => el("option", { value: v }, v || "Все события")));
    vybor.value = filtr.vazhnost;
    vybor.onchange = () => { filtr.vazhnost = vybor.value; sost.aktivnaya.risovat(forma); };
    forma.append(el("h1", {}, "Журнал регистрации"), el("div", { class: "panel-knopok" }, el("span", {}, "Важность:"), vybor,
      el("button", { onclick: () => sost.aktivnaya.risovat(forma) }, "Обновить")));
    let d;
    try { d = await api(`/api/${b(sost.baza)}/zhurnal`); } catch { return; }
    const t = el("table", { class: "spisok" }, el("tr", {}, ["", "Дата", "Событие", "Метаданные", "Данные", "Комментарий"].map((k) => el("th", {}, k))));
    for (const z of d || []) {
      if (filtr.vazhnost && z.Важность !== filtr.vazhnost) continue;
      t.append(el("tr", {}, el("td", { class: "znachok" }, z.Важность === "Ошибка" ? el("span", { class: "pomechen" }, "!") : el("span", { class: "zapisan" }, "i")),
        el("td", {}, z.Дата), el("td", {}, z.Событие), el("td", {}, z.Метаданные), el("td", {}, z.Данные), el("td", { style: "white-space:normal" }, z.Комментарий)));
    }
    forma.append(t);
  });
}

function pomechennye() {
  vkladka("pomechennye", "Удаление помеченных", async (forma) => {
    forma.innerHTML = "";
    forma.append(el("h1", {}, "Удаление помеченных объектов"),
      el("div", { class: "otchet-podzagolovok" }, "Удаляет обработка «Удаление помеченных» раз в час, если на объект нет ссылок. Здесь — что сейчас помечено."));
    const t = el("table", { class: "spisok" }, el("tr", {}, el("th", { class: "znachok" }, ""), el("th", {}, "Вид"), el("th", {}, "Подразделение"), el("th", {}, "Наименование")));
    let est = false;
    for (const vid of ["Справочники", "Секреты", "ЗаявкиНаТом", "Услуги", "Развёртывания", "НаборыСоСостоянием", "ПодотчётныеЛица"]) {
      let d;
      try { d = await api(`/api/${b(sost.baza)}/spisok/${b(vid)}`); } catch { continue; }
      for (const s of d.Строки || []) if (s.Пометка) {
        est = true;
        t.append(el("tr", { ondblclick: () => obekt(vid, s.Подразделение || "-", s.ИмяAPI || s.Имя, s.Имя) },
          el("td", { class: "znachok" }, el("span", { class: "pomechen" }, "✗")), el("td", {}, raskryt(vid)), el("td", {}, s.Подразделение), el("td", {}, s.Имя)));
      }
    }
    forma.append(est ? t : el("div", { class: "pusto" }, "Помеченных на удаление объектов нет. Чистота, как в кассе после инкассации."));
  });
}

// ——— Девочки, календарь, обед ———

async function golos() {
  let d;
  try { d = await api("/golos/boltovnya"); } catch { return; }
  const lenta = $("lenta");
  if (d.Фраза) lenta.prepend(el("div", { class: "soobshchenie" }, el("b", {}, d.Персона), d.Фраза));
  if (d.Календарь && !lenta.dataset.kalendar) { lenta.dataset.kalendar = "1"; lenta.prepend(el("div", { class: "soobshchenie" }, el("b", {}, "Тамара Петровна"), d.Календарь)); }
  while (lenta.children.length > 30) lenta.lastChild.remove();
  $("obed").hidden = !d.Обед;
}
$("knopka-devochki").onclick = () => { $("devochki").hidden = !$("devochki").hidden; };
$("zakryt-devochek").onclick = () => { $("devochki").hidden = true; };

document.addEventListener("keydown", (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key === "f" && !$("prilozhenie").hidden) { e.preventDefault(); $("poisk").focus(); }
});
$("poisk").addEventListener("keydown", (e) => {
  if (e.key !== "Enter") return;
  const q = $("poisk").value.trim();
  if (!q) return;
  vkladka("poisk:" + q, "Поиск: " + q, async (forma) => {
    forma.innerHTML = "";
    forma.append(el("h1", {}, "Поиск: " + q));
    await tablicaZaprosa(forma, `ВЫБРАТЬ П.Подразделение, П.Имя, П.Фаза ИЗ Кластер.Поды КАК П ГДЕ П.Имя ПОДОБНО "%${q.replace(/"/g, "")}%" УПОРЯДОЧИТЬ ПО П.Имя`);
  });
});

zapusk().catch(() => {});
