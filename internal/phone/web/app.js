// i-mux on a phone (D104, D110): pairs from the QR code's fragment, then
// talks to imux with every message sealed (AES-256-GCM, the key from the QR
// code, the device's id as additional data). No inline code: the page's CSP
// allows only these files. The desktop is never changed by what the phone
// shows: it reads the agents and a pane's screen, and types into it.
import { gcm } from "/noble-aes.js";
import { choices } from "/choices.js";

const TR = (navigator.language || "").toLowerCase().startsWith("tr");
const T = {
  pair_title: TR ? "Bu telefonu eşleştir" : "Pair this phone",
  step1: TR ? "Bilgisayarda i-mux'un kenar panelindeki <b>Telefon</b>'a tıklayın." : "On your computer, click <b>Phone</b> on i-mux's sidebar.",
  step2: TR ? "<b>Telefon eşleştir</b>'i seçin: bir QR kod çıkar." : "Choose <b>Pair a phone</b>: a QR code shows.",
  step3: TR ? "QR kodu bu telefonun kamerasıyla okutun." : "Scan it with this phone's camera.",
  read_only: TR ? "Bu telefon sadece izliyor." : "This phone only watches.",
  no_agents: TR ? "Açık agent yok." : "No agents are running.",
  no_agents_hint: TR ? "Bilgisayarda bir agent başlatınca burada görünür." : "Start an agent on your computer and it shows here.",
  working: TR ? "çalışıyor" : "working",
  blocked: TR ? "seni bekliyor" : "needs you",
  idle: TR ? "boşta" : "idle",
  needs_you: TR ? "Seni bekleyenler" : "Needs you",
  waits: TR ? "%s seni bekliyor" : "%s needs you",
  now: TR ? "şimdi" : "now",
  min: TR ? "dk" : "m",
  hour: TR ? "sa" : "h",
  day: TR ? "g" : "d",
  earlier: TR ? "Önceki satırlar" : "Earlier lines",
  paired_failed: TR ? "Eşleştirme olmadı: " : "Pairing failed: ",
  forgotten: TR ? "Bu telefon artık eşli değil. Yeniden eşleştirin." : "This phone is not paired any more. Pair it again.",
  offline: TR ? "Bağlantı yok, yeniden deneniyor…" : "Offline, trying again…",
  send_offline: TR ? "Gönderilemedi: bağlantı yok" : "Not sent: offline",
  send_readonly: TR ? "Bu telefon sadece izliyor" : "This phone only watches",
  send_gone: TR ? "Bu agent artık yok" : "This agent is gone",
  send_failed: TR ? "Gönderilemedi" : "Not sent",
  prompt_add: TR ? "Önce kutuya yazın, sonra + ile kaydedin" : "Type it in the box first, then + saves it",
  prompt_saved: TR ? "Kaydedildi" : "Saved",
  prompt_remove: TR ? "“%s” silinsin mi?" : "Remove “%s”?",
  fit: TR ? "Ekrana sığdırıldı" : "Fit to the screen",
  tip_ios: TR ? "Uygulama gibi açmak için: Paylaş → <b>Ana Ekrana Ekle</b>." : "To open it like an app: Share → <b>Add to Home Screen</b>.",
  tip_other: TR ? "Uygulama gibi açmak için: tarayıcı menüsü → <b>Uygulamayı yükle</b> (ya da Ana ekrana ekle)." : "To open it like an app: the browser's menu → <b>Install app</b> (or Add to Home screen).",
  ok: TR ? "Tamam" : "OK",
  message: TR ? "Mesaj" : "Message",
};
const $ = (s) => document.querySelector(s);
const el = (tag, cls, text) => {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
};
const tap = () => navigator.vibrate && navigator.vibrate(8);

// --- keys and sealing -------------------------------------------------
const enc = new TextEncoder();
const dec = new TextDecoder();
function b64uDecode(s) {
  s = s.replace(/-/g, "+").replace(/_/g, "/");
  while (s.length % 4) s += "=";
  return Uint8Array.from(atob(s), (c) => c.charCodeAt(0));
}
function b64uEncode(b) {
  let s = "";
  for (const x of b) s += String.fromCharCode(x);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}
const KEY = "imux-phone";
function saved() {
  try { return JSON.parse(localStorage.getItem(KEY)); } catch { return null; }
}
let me = saved(); // {id, key}
let key = null, aad = null;
function useDevice(d) {
  me = d;
  key = b64uDecode(d.key);
  aad = enc.encode(d.id);
}
function seal(obj) {
  obj.ts = Date.now();
  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const ct = gcm(key, nonce, aad).encrypt(enc.encode(JSON.stringify(obj)));
  const out = new Uint8Array(12 + ct.length);
  out.set(nonce);
  out.set(ct, 12);
  return b64uEncode(out);
}
function unseal(s) {
  const b = b64uDecode(s);
  return JSON.parse(dec.decode(gcm(key, b.subarray(0, 12), aad).decrypt(b.subarray(12))));
}

// --- pairing ----------------------------------------------------------
async function pairFromFragment() {
  const m = location.hash.match(/^#p=([A-Za-z0-9_-]+)\.([A-Za-z0-9_-]+)$/);
  if (!m) return;
  history.replaceState(null, "", location.pathname); // the key leaves the address bar
  const name = (navigator.userAgentData && navigator.userAgentData.platform) ||
    (/iPhone|iPad|Android/.exec(navigator.userAgent) || ["Phone"])[0];
  try {
    const r = await fetch("/api/pair", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token: m[1], name }),
    });
    if (!r.ok) throw new Error((await r.text()).trim());
    const { id } = await r.json();
    const d = { id, key: m[2] };
    localStorage.setItem(KEY, JSON.stringify(d));
    me = d;
  } catch (e) {
    const p = $("#pair-error");
    p.textContent = T.paired_failed + e.message;
    p.hidden = false;
  }
}


// --- the desktop's theme (D110) -------------------------------------------
let look = null;
function applyTheme() {
  const root = document.documentElement.style;
  for (let i = root.length - 1; i >= 0; i--) if (root[i].startsWith("--t-")) root.removeProperty(root[i]);
  if (!look) return;
  const light = look.light && matchMedia("(prefers-color-scheme: light)").matches;
  const colors = (light ? look.light : look.colors) || {};
  for (const [k, v] of Object.entries(colors)) root.setProperty("--t-" + k, v);
  const bg = colors.sidebar_bg || colors.panel_bg;
  if (bg) document.querySelector('meta[name="theme-color"]').content = bg;
  if (colors.accent) root.setProperty("--on-accent", luma(colors.accent) > 0.55 ? "#0b0e13" : "#ffffff");
  const term = colors.sidebar_bg || colors.panel_bg;
  if (term && luma(term) > 0.6) { root.setProperty("--term-bg", term); root.setProperty("--term-fg", colors.text || "#24292f"); }
  else { root.removeProperty("--term-bg"); root.removeProperty("--term-fg"); }
}
function luma(hex) {
  const n = parseInt(hex.slice(1), 16);
  return (0.299 * (n >> 16) + 0.587 * ((n >> 8) & 255) + 0.114 * (n & 255)) / 255;
}
matchMedia("(prefers-color-scheme: light)").addEventListener("change", applyTheme);

// --- the stream ---------------------------------------------------------
let es = null, retry = null, readOnly = false;
let projects = [], watching = null, lines = 0;
const states = new Map(); // pane → its last state, to see one start waiting
function setStatus(on) {
  const d = $("#status");
  d.className = "dot " + (on ? "on" : "off");
  d.title = on ? "" : T.offline;
  $("#offline").hidden = on || !me;
}
function connect() {
  clearTimeout(retry);
  if (es) es.close();
  const u = "/api/events?d=" + encodeURIComponent(me.id) + "&a=" + encodeURIComponent(seal({ op: "events" }));
  es = new EventSource(u);
  $("#status").hidden = false;
  es.onopen = () => setStatus(true);
  es.onmessage = (e) => {
    let m;
    try { m = unseal(e.data); } catch { return; }
    if (m.t === "hello") {
      readOnly = !!m.read_only;
      look = m.theme || null;
      applyTheme();
      if (watching) { watch(); showPane(); }
    } else if (m.t === "agents") {
      projects = m.projects || [];
      noticeWaiting();
      if (watching) updatePaneTitle(); else renderList();
    } else if (m.t === "screen" && watching && m.pane === watching.pane) {
      renderScreen(m.screen);
    }
  };
  es.onerror = async () => {
    setStatus(false);
    es.close();
    // A forgotten phone: the server refuses it; anything else, try again.
    try {
      const r = await fetch("/api/events?d=" + encodeURIComponent(me.id) + "&a=x");
      if (r.status === 401 && !(await r.text()).includes("refused")) return forgotten();
    } catch {}
    retry = setTimeout(connect, 2000);
  };
}
function forgotten() {
  localStorage.removeItem(KEY);
  me = null;
  showPair(T.forgotten);
}
async function cmd(obj) {
  try {
    const r = await fetch("/api/cmd", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ d: me.id, m: seal(obj) }),
    });
    return r.status;
  } catch { return 0; }
}
function watch() { return cmd({ op: "watch", pane: watching.pane, lines }); }

// --- agents ---------------------------------------------------------------
const MARK = { claude: "Cl", antigravity: "Ag", copilot: "Cp", codex: "Cx", opencode: "Oc" };
const CLI = { claude: "Claude", antigravity: "Antigravity", copilot: "Copilot", codex: "Codex", opencode: "OpenCode" };
function label(a) { return a.name || CLI[a.cli] || a.cli; }
function stateOf(a) { return a.state === "working" || a.state === "blocked" ? a.state : "idle"; }
function ago(since) {
  if (!since) return "";
  const s = Math.max(0, Date.now() / 1000 - since);
  if (s < 60) return T.now;
  if (s < 3600) return Math.floor(s / 60) + T.min;
  if (s < 86400) return Math.floor(s / 3600) + T.hour;
  return Math.floor(s / 86400) + T.day;
}
function stateText(a) {
  const t = ago(a.since);
  return T[stateOf(a)] + (t ? " · " + t : "");
}
function all() {
  const out = [];
  for (const p of projects) for (const a of p.agents) out.push({ p, a });
  return out;
}
function find(pane) { return all().find((x) => x.a.pane === pane); }

// An agent that starts waiting for you while you look elsewhere: a banner,
// a buzz, and the count in the title.
let bannerPane = null;
function noticeWaiting() {
  let waiting = 0;
  for (const { p, a } of all()) {
    const st = stateOf(a);
    if (st === "blocked") waiting++;
    const was = states.get(a.pane);
    states.set(a.pane, st);
    if (st === "blocked" && was && was !== "blocked" && (!watching || watching.pane !== a.pane || document.hidden)) {
      bannerPane = a.pane;
      const b = $("#banner");
      b.textContent = T.waits.replace("%s", label(a) + " · " + p.name);
      b.hidden = false;
      navigator.vibrate && navigator.vibrate([60, 60, 60]);
    }
  }
  if (bannerPane && !(find(bannerPane) && stateOf(find(bannerPane).a) === "blocked")) hideBanner();
  document.title = waiting ? `(${waiting}) i-mux` : "i-mux";
}
function hideBanner() { bannerPane = null; $("#banner").hidden = true; }
$("#banner").onclick = () => {
  const x = bannerPane && find(bannerPane);
  hideBanner();
  if (x) openAgent(x.p, x.a);
};

// --- views --------------------------------------------------------------
function only(id) {
  for (const s of ["#pair", "#list", "#pane"]) $(s).hidden = s !== id;
  $("#controls").hidden = $("#watching").hidden = true;
}
function setTitle(title, sub) {
  $("#title").textContent = title;
  const s = $("#subtitle");
  s.textContent = sub || "";
  s.hidden = !sub;
}
function showPair(err) {
  only("#pair");
  $("#status").hidden = true; // nothing to be connected to yet
  $("#back").hidden = true;
  setTitle("i-mux");
  if (err) { const p = $("#pair-error"); p.textContent = err; p.hidden = false; }
}
function agentRow(p, a, withProject) {
  const st = stateOf(a);
  const row = el("button", "agent st-" + st);
  const mark = el("span", "mark " + a.cli, MARK[a.cli] || (a.cli || "?").slice(0, 2));
  const body = el("span", "body");
  const name = el("span", "name", label(a));
  if (a.name) name.append(el("small", "", CLI[a.cli] || a.cli));
  else if (withProject) name.append(el("small", "", p.name));
  body.append(name, el("span", "sub", stateText(a)));
  row.append(mark, body, el("span", "chev", "›"));
  row.onclick = () => { tap(); openAgent(p, a); };
  return row;
}
function renderList() {
  const list = $("#list");
  only("#list");
  $("#back").hidden = true;
  setTitle("i-mux");
  const scroll = list.scrollTop;
  list.replaceChildren();
  const everyone = all();
  if (!everyone.length) {
    const e = el("div", "empty");
    e.append(el("span", "big", "⌁"), el("div", "", T.no_agents), el("small", "", T.no_agents_hint));
    list.append(e);
    return;
  }
  const count = { blocked: 0, working: 0, idle: 0 };
  for (const { a } of everyone) count[stateOf(a)]++;
  const sum = el("div", "summary");
  for (const st of ["blocked", "working", "idle"]) {
    if (!count[st]) continue;
    const c = el("span", "chip " + st);
    c.append(el("i"), el("b", "", String(count[st])), document.createTextNode(T[st]));
    sum.append(c);
  }
  list.append(sum);
  if (showTip()) list.prepend(tip());
  const waiting = everyone.filter(({ a }) => stateOf(a) === "blocked");
  if (waiting.length) {
    const g = el("div", "group attention");
    g.append(el("h3", "", T.needs_you));
    for (const { p, a } of waiting) g.append(agentRow(p, a, true));
    list.append(g);
  }
  for (const p of projects) {
    if (!p.agents.length) continue;
    const box = el("div", "project");
    const h = el("div", "phead");
    h.append(el("span", "pname", p.name));
    if (p.branch) h.append(el("span", "branch", "⎇ " + p.branch));
    h.append(el("span", "count", String(p.agents.length)));
    box.append(h);
    for (const a of p.agents) box.append(agentRow(p, a, false));
    list.append(box);
  }
  list.scrollTop = scroll;
}
function openAgent(p, a) {
  if (bannerPane === a.pane) hideBanner();
  const first = !watching;
  watching = { pane: a.pane };
  lines = 0;
  lastText = "";
  $("#screen").replaceChildren();
  $("#choices").hidden = true;
  stick = true;
  watch();
  showPane();
  if (first) history.pushState({ pane: a.pane }, "");
}
function showPane() {
  only("#pane");
  $("#back").hidden = false;
  $("#earlier").hidden = lines >= 500;
  $("#controls").hidden = readOnly;
  $("#watching").hidden = !readOnly;
  updatePaneTitle();
}
function updatePaneTitle() {
  const x = watching && find(watching.pane);
  if (!x) { setTitle("i-mux", ""); return; }
  const list = all();
  const at = list.findIndex((y) => y.a.pane === watching.pane);
  setTitle(label(x.a), x.p.name + " · " + stateText(x.a) + (list.length > 1 ? " · " + (at + 1) + "/" + list.length : ""));
}
function leave() {
  watching = null;
  lines = 0;
  $("#screen").replaceChildren();
  renderList();
}
$("#back").onclick = () => history.back();
addEventListener("popstate", () => { if (watching) leave(); });
setInterval(() => { if (!document.hidden && projects.length) { if (watching) updatePaneTitle(); else renderList(); } }, 30000);

// --- the screen: SGR colours into spans (no inline styles in HTML; the
// CSSOM is allowed by the CSP) --------------------------------------------
const BASE = ["#484f58", "#ff7b72", "#3fb950", "#d29922", "#58a6ff", "#bc8cff", "#39c5cf", "#b1bac4",
  "#6e7681", "#ffa198", "#56d364", "#e3b341", "#79c0ff", "#d2a8ff", "#56d4dd", "#ffffff"];
function c256(n) {
  if (n < 16) return BASE[n];
  if (n < 232) { n -= 16; const l = [0, 95, 135, 175, 215, 255]; return `rgb(${l[Math.floor(n / 36)]},${l[Math.floor(n / 6) % 6]},${l[n % 6]})`; }
  const g = 8 + (n - 232) * 10; return `rgb(${g},${g},${g})`;
}
const FONT = "imux-phone-font";
let scale = Math.min(2.5, Math.max(0.5, parseFloat(localStorage.getItem(FONT)) || 1));
let lastScreen = null, lastText = "", stick = true;
function fontSize(sc) {
  const wrap = $("#screen-wrap");
  const fit = (wrap.clientWidth - 16) / (Math.max(sc.w, 20) * 0.6);
  return Math.max(6, Math.min(22, fit * scale));
}
function renderScreen(sc) {
  lastScreen = sc;
  const pre = $("#screen");
  const wrap = $("#screen-wrap");
  pre.style.fontSize = fontSize(sc) + "px";
  const frag = document.createDocumentFragment();
  let st = {};
  let buf = "";
  const flush = () => {
    if (!buf) return;
    const s = document.createElement("span");
    let fg = st.fg, bg = st.bg;
    if (st.rev) [fg, bg] = [bg || "var(--term-bg)", fg || "var(--term-fg)"];
    if (fg) s.style.color = fg;
    if (bg) s.style.backgroundColor = bg;
    if (st.bold) s.style.fontWeight = "700";
    if (st.dim) s.style.opacity = "0.6";
    if (st.italic) s.style.fontStyle = "italic";
    if (st.under) s.style.textDecoration = "underline";
    s.textContent = buf;
    frag.append(s);
    buf = "";
  };
  const re = /\x1b\[([0-9;:]*)m|\x1b\[[0-9;?]*[A-Za-z]|\x1b[()][A-Za-z0-9]|([^\x1b]+)/g;
  let m;
  while ((m = re.exec(sc.text))) {
    if (m[2] !== undefined) { buf += m[2]; continue; }
    if (m[1] === undefined) continue;
    flush();
    const p = m[1] === "" ? [0] : m[1].split(/[;:]/).map((x) => parseInt(x || "0", 10));
    for (let i = 0; i < p.length; i++) {
      const c = p[i];
      if (c === 0) st = {};
      else if (c === 1) st.bold = true;
      else if (c === 2) st.dim = true;
      else if (c === 3) st.italic = true;
      else if (c === 4) st.under = true;
      else if (c === 7) st.rev = true;
      else if (c === 22) st.bold = st.dim = false;
      else if (c === 23) st.italic = false;
      else if (c === 24) st.under = false;
      else if (c === 27) st.rev = false;
      else if (c >= 30 && c <= 37) st.fg = BASE[c - 30];
      else if (c >= 90 && c <= 97) st.fg = BASE[c - 82];
      else if (c >= 40 && c <= 47) st.bg = BASE[c - 40];
      else if (c >= 100 && c <= 107) st.bg = BASE[c - 92];
      else if (c === 39) st.fg = undefined;
      else if (c === 49) st.bg = undefined;
      else if ((c === 38 || c === 48) && p[i + 1] === 5) { st[c === 38 ? "fg" : "bg"] = c256(p[i + 2] || 0); i += 2; }
      else if ((c === 38 || c === 48) && p[i + 1] === 2) { st[c === 38 ? "fg" : "bg"] = `rgb(${p[i + 2] | 0},${p[i + 3] | 0},${p[i + 4] | 0})`; i += 4; }
    }
  }
  flush();
  pre.replaceChildren(frag);
  if (stick) wrap.scrollTop = wrap.scrollHeight;
  if (sc.text !== lastText) { lastText = sc.text; renderChoices(sc.text); }
}
$("#screen-wrap").addEventListener("scroll", () => {
  const w = $("#screen-wrap");
  stick = w.scrollTop + w.clientHeight >= w.scrollHeight - 24;
  $("#bottom").hidden = stick;
}, { passive: true });
$("#bottom").onclick = () => { const w = $("#screen-wrap"); stick = true; w.scrollTop = w.scrollHeight; };
function rescale(by) {
  tap();
  scale = Math.min(2.5, Math.max(0.5, Math.round(scale * by * 100) / 100));
  localStorage.setItem(FONT, String(scale));
  if (lastScreen) renderScreen(lastScreen);
}
$("#smaller").onclick = () => rescale(1 / 1.15);
$("#larger").onclick = () => rescale(1.15);
$("#earlier").onclick = () => {
  tap();
  lines = Math.min(500, lines + 200);
  $("#earlier").hidden = lines >= 500;
  stick = false;
  watch();
};
addEventListener("resize", () => { if (lastScreen && watching) renderScreen(lastScreen); });

// Quick answers: the agent's numbered choices as large buttons.
function renderChoices(text) {
  const box = $("#choices");
  const cs = readOnly ? [] : choices(text);
  box.replaceChildren();
  box.hidden = !cs.length;
  for (const c of cs) {
    const b = el("button", c.current ? "current" : "");
    b.append(el("b", "", String(c.n)), el("span", "", c.label));
    b.onclick = () => { tap(); send({ keys: [String(c.n)] }); };
    box.append(b);
  }
}

// --- input ----------------------------------------------------------------
async function send(what) {
  if (!watching) return;
  stick = true;
  const code = await cmd(Object.assign({ op: "send", pane: watching.pane }, what));
  if (code === 204) return;
  toast(code === 0 ? T.send_offline : code === 403 ? T.send_readonly : code === 404 ? T.send_gone : T.send_failed);
}
let toastTimer = null;
function toast(msg) {
  const t = $("#toast");
  t.textContent = msg;
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, 2200);
}
for (const b of document.querySelectorAll("[data-key]")) {
  b.onclick = () => { tap(); send({ keys: [b.dataset.key] }); };
}
const text = $("#text");
function grow() { text.style.height = "auto"; text.style.height = Math.min(text.scrollHeight, 132) + "px"; }
text.addEventListener("input", grow);
text.addEventListener("keydown", (e) => {
  // Enter sends; Shift+Enter (a hardware keyboard) is a new line.
  if (e.key === "Enter" && !e.shiftKey && !e.isComposing) { e.preventDefault(); $("#say").requestSubmit(); }
});
$("#say").onsubmit = (e) => {
  e.preventDefault();
  if (!watching) return;
  tap();
  if (!text.value) { send({ keys: ["Enter"] }); return; }
  send({ text: text.value.replace(/\r?\n/g, " "), keys: ["Enter"] });
  text.value = "";
  grow();
};


// --- D111: quick prompts --------------------------------------------------
const PROMPTS = "imux-phone-prompts";
const DEFAULTS = TR
  ? ["Devam et", "Testleri çalıştır", "Ne yaptığını kısaca özetle", "Değişiklikleri commit et"]
  : ["Continue", "Run the tests", "Summarize what you did", "Commit the changes"];
function prompts() {
  try { const p = JSON.parse(localStorage.getItem(PROMPTS)); if (Array.isArray(p)) return p; } catch {}
  return DEFAULTS.slice();
}
function savePrompts(p) { localStorage.setItem(PROMPTS, JSON.stringify(p.slice(0, 30))); renderPrompts(); }
function renderPrompts() {
  const box = $("#prompts");
  box.replaceChildren();
  for (const p of prompts()) {
    const b = el("button", "", p);
    let held = null, long = false;
    b.addEventListener("pointerdown", () => {
      long = false;
      held = setTimeout(() => {
        long = true;
        navigator.vibrate && navigator.vibrate(20);
        if (confirm(T.prompt_remove.replace("%s", p))) savePrompts(prompts().filter((x) => x !== p));
      }, 550);
    });
    for (const ev of ["pointerup", "pointerleave", "pointercancel"]) b.addEventListener(ev, () => clearTimeout(held));
    b.addEventListener("contextmenu", (e) => e.preventDefault());
    b.onclick = () => { if (long) return; tap(); send({ text: p, keys: ["Enter"] }); };
    box.append(b);
  }
  const add = el("button", "add", "+");
  add.setAttribute("aria-label", T.prompt_saved);
  add.onclick = () => {
    const v = text.value.trim().replace(/\s+/g, " ");
    if (!v) return toast(T.prompt_add);
    if (!prompts().includes(v)) savePrompts([...prompts(), v]);
    tap();
    toast(T.prompt_saved);
  };
  box.append(add);
}

// --- D111: swipe to the next or previous agent ------------------------------
let touch = null;
function onTouchStart(e) {
  const t = e.touches[0];
  touch = e.touches.length === 1 ? { x: t.clientX, y: t.clientY, at: Date.now(), header: !!e.target.closest("header") } : null;
}
function onTouchEnd(e) {
  if (!touch || !watching) return;
  const t = e.changedTouches[0];
  const dx = t.clientX - touch.x, dy = t.clientY - touch.y;
  const quick = Date.now() - touch.at < 600;
  const wrap = $("#screen-wrap");
  const flat = wrap.scrollWidth <= wrap.clientWidth + 2;
  const from = touch;
  touch = null;
  if (!quick || Math.abs(dx) < 70 || Math.abs(dy) > 45 || !(from.header || flat)) return;
  const list = all();
  const at = list.findIndex((y) => y.a.pane === watching.pane);
  if (at < 0 || list.length < 2) return;
  const next = list[(at + (dx < 0 ? 1 : -1) + list.length) % list.length];
  const pane = $("#pane");
  pane.classList.remove("slide-l", "slide-r");
  void pane.offsetWidth;
  pane.classList.add(dx < 0 ? "slide-l" : "slide-r");
  tap();
  openAgent(next.p, next.a);
}
for (const t of [$("#pane"), $("header")]) {
  t.addEventListener("touchstart", onTouchStart, { passive: true });
  t.addEventListener("touchend", onTouchEnd, { passive: true });
}

// --- D111: a double tap fits the screen again -------------------------------
let lastTap = 0;
$("#screen-wrap").addEventListener("touchend", (e) => {
  if (e.changedTouches.length !== 1) return;
  const now = Date.now();
  if (now - lastTap < 300) {
    lastTap = 0;
    if (scale !== 1) {
      scale = 1;
      localStorage.setItem(FONT, "1");
      if (lastScreen) renderScreen(lastScreen);
      toast(T.fit);
    }
  } else lastTap = now;
}, { passive: true });

// --- D111: the home-screen tip, once ----------------------------------------
const TIP = "imux-phone-tip";
function standalone() {
  return navigator.standalone === true || matchMedia("(display-mode: standalone)").matches;
}
function showTip() { return !standalone() && !localStorage.getItem(TIP); }
function tip() {
  const box = el("div", "tip");
  const s = el("span");
  s.innerHTML = /iPhone|iPad|iPod/.test(navigator.userAgent) ? T.tip_ios : T.tip_other; // our own text, with <b>
  const ok = el("button", "", T.ok);
  ok.onclick = () => { localStorage.setItem(TIP, "1"); box.remove(); };
  box.append(s, ok);
  return box;
}

// --- start ----------------------------------------------------------------
for (const e of document.querySelectorAll("[data-t]")) {
  const k = e.dataset.t;
  if (k.startsWith("step")) e.innerHTML = T[k]; // our own text, with <b>
  else e.textContent = T[k];
}
text.placeholder = T.message;
renderPrompts();
await pairFromFragment();
if (me && me.id && me.key) {
  useDevice(me);
  connect();
} else {
  showPair();
}
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible" && me && key) connect();
});
