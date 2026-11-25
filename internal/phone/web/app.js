// i-mux on a phone (D104): pairs from the QR code's fragment, then talks to
// imux with every message sealed (AES-256-GCM, the key from the QR code,
// the device's id as additional data). No inline code: the page's CSP
// allows only these files.
import { gcm } from "/noble-aes.js";

const TR = (navigator.language || "").toLowerCase().startsWith("tr");
const T = {
  pair_title: TR ? "Bu telefonu eşleştir" : "Pair this phone",
  pair_body: TR
    ? "Bilgisayarda i-mux'ta kenar paneldeki <b>Phone</b>'a tıklayıp <b>Telefon eşleştir</b>'i seçin. Sonra QR kodu bu telefonun kamerasıyla okutun."
    : "On your computer, open i-mux, click <b>Phone</b> on the sidebar and choose <b>Pair a phone</b>. Then scan the QR code with this phone's camera.",
  send: TR ? "Gönder" : "Send",
  read_only: TR ? "Bu telefon sadece izliyor." : "This phone only watches.",
  no_agents: TR ? "Açık agent yok." : "No agents are running.",
  working: TR ? "çalışıyor" : "working",
  blocked: TR ? "seni bekliyor" : "needs you",
  idle: TR ? "boşta" : "idle",
  paired_failed: TR ? "Eşleştirme olmadı: " : "Pairing failed: ",
  forgotten: TR ? "Bu telefon artık eşli değil. Yeniden eşleştirin." : "This phone is not paired any more. Pair it again.",
  message: TR ? "Mesaj" : "Message",
};
const $ = (s) => document.querySelector(s);

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

// --- the stream ---------------------------------------------------------
let es = null, retry = null, readOnly = false;
let projects = [], watching = null;
function setStatus(on) {
  const d = $("#status");
  d.className = "dot " + (on ? "on" : "off");
}
function connect() {
  clearTimeout(retry);
  if (es) es.close();
  const u = "/api/events?d=" + encodeURIComponent(me.id) + "&a=" + encodeURIComponent(seal({ op: "events" }));
  es = new EventSource(u);
  es.onopen = () => setStatus(true);
  es.onmessage = (e) => {
    let m;
    try { m = unseal(e.data); } catch { return; }
    if (m.t === "hello") {
      readOnly = !!m.read_only;
      if (watching) { cmd({ op: "watch", pane: watching.pane }); showPane(watching); }
    } else if (m.t === "agents") {
      projects = m.projects || [];
      if (!watching) renderList();
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

// --- views --------------------------------------------------------------
function showPair(err) {
  $("#pair").hidden = false;
  $("#list").hidden = $("#pane").hidden = $("#controls").hidden = $("#watching").hidden = true;
  $("#back").hidden = true;
  $("#title").textContent = "i-mux";
  if (err) { const p = $("#pair-error"); p.textContent = err; p.hidden = false; }
}
function label(a) { return a.name || a.cli; }
function renderList() {
  const el = $("#list");
  $("#pair").hidden = $("#pane").hidden = $("#controls").hidden = $("#watching").hidden = true;
  $("#back").hidden = true;
  $("#title").textContent = "i-mux";
  el.hidden = false;
  el.replaceChildren();
  const any = projects.some((p) => p.agents.length);
  if (!any) {
    const e = document.createElement("p");
    e.className = "empty";
    e.textContent = T.no_agents;
    el.append(e);
    return;
  }
  for (const p of projects) {
    if (!p.agents.length) continue;
    const box = document.createElement("div");
    box.className = "project";
    const h = document.createElement("h3");
    h.textContent = p.name;
    if (p.branch) {
      const b = document.createElement("span");
      b.className = "branch";
      b.textContent = "⎇ " + p.branch;
      h.append(b);
    }
    box.append(h);
    for (const a of p.agents) {
      const row = document.createElement("button");
      row.className = "agent";
      const n = document.createElement("span");
      n.className = "name";
      n.textContent = label(a);
      if (a.name) {
        const c = document.createElement("span");
        c.className = "cli";
        c.textContent = a.cli;
        n.append(c);
      }
      const st = a.state === "working" || a.state === "blocked" ? a.state : "idle";
      const badge = document.createElement("span");
      badge.className = "badge " + st;
      badge.textContent = T[st];
      row.append(n, badge);
      row.onclick = () => { watching = { pane: a.pane, title: p.name + " · " + label(a) }; cmd({ op: "watch", pane: a.pane }); showPane(watching); };
      box.append(row);
    }
    el.append(box);
  }
}
function showPane(w) {
  $("#pair").hidden = $("#list").hidden = true;
  $("#pane").hidden = false;
  $("#back").hidden = false;
  $("#title").textContent = w.title;
  $("#controls").hidden = readOnly;
  $("#watching").hidden = !readOnly;
}
$("#back").onclick = () => { watching = null; $("#screen").replaceChildren(); renderList(); };

// --- the screen: SGR colours into spans (no inline styles in HTML; the
// CSSOM is allowed by the CSP) --------------------------------------------
const BASE = ["#484f58", "#ff7b72", "#3fb950", "#d29922", "#58a6ff", "#bc8cff", "#39c5cf", "#b1bac4",
  "#6e7681", "#ffa198", "#56d364", "#e3b341", "#79c0ff", "#d2a8ff", "#56d4dd", "#ffffff"];
function c256(n) {
  if (n < 16) return BASE[n];
  if (n < 232) { n -= 16; const l = [0, 95, 135, 175, 215, 255]; return `rgb(${l[Math.floor(n / 36)]},${l[Math.floor(n / 6) % 6]},${l[n % 6]})`; }
  const g = 8 + (n - 232) * 10; return `rgb(${g},${g},${g})`;
}
function renderScreen(sc) {
  const pre = $("#screen");
  const wrap = $("#screen-wrap");
  const fit = Math.max(7, Math.min(14, (wrap.clientWidth - 16) / (Math.max(sc.w, 20) * 0.6)));
  pre.style.fontSize = fit + "px";
  const frag = document.createDocumentFragment();
  let st = {};
  let buf = "";
  const flush = () => {
    if (!buf) return;
    const s = document.createElement("span");
    let fg = st.fg, bg = st.bg;
    if (st.rev) [fg, bg] = [bg || "#000", fg || "#d0d7de"];
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
  const atBottom = wrap.scrollTop + wrap.clientHeight >= wrap.scrollHeight - 4;
  pre.replaceChildren(frag);
  if (atBottom) wrap.scrollTop = wrap.scrollHeight;
}

// --- input ----------------------------------------------------------------
for (const b of document.querySelectorAll("[data-key]")) {
  b.onclick = () => watching && cmd({ op: "send", pane: watching.pane, keys: [b.dataset.key] });
}
$("#say").onsubmit = (e) => {
  e.preventDefault();
  const t = $("#text");
  if (!watching || !t.value) return;
  cmd({ op: "send", pane: watching.pane, text: t.value, keys: ["Enter"] });
  t.value = "";
};

// --- start ----------------------------------------------------------------
for (const el of document.querySelectorAll("[data-t]")) {
  const k = el.dataset.t;
  if (k === "pair_body") el.innerHTML = T[k]; // our own text, with <b>
  else el.textContent = T[k];
}
$("#text").placeholder = T.message;
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
