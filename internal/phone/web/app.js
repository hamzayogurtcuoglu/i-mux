// i-mux on a phone (D104, D105): the workspace itself, drawn by xterm.js from
// the engine's terminal output; taps are the mouse, the keyboard and the key
// bar are the keys. Every message both ways is sealed with the key from the
// QR code (AES-256-GCM, the device's id as additional data).
import { Terminal } from "/xterm.mjs";
import { gcm } from "/noble-aes.js";

const $ = (s) => document.querySelector(s);
const TR = (navigator.language || "").toLowerCase().startsWith("tr");
const T = {
  pair_title: TR ? "Bu telefonu eşleştir" : "Pair this phone",
  pair_1: TR ? "Bilgisayarda i-mux kenar panelindeki <b>Phone</b>'a tıklayın." : "On your computer, click <b>Phone</b> on i-mux's sidebar.",
  pair_2: TR ? "<b>p</b> yazıp Enter'a basın: bir QR kod çıkar." : "Type <b>p</b> and Enter: a QR code appears.",
  pair_3: TR ? "QR kodu bu telefonun kamerasıyla okutun." : "Scan it with this phone's camera.",
  pair_failed: TR ? "Eşleştirme olmadı: " : "Pairing failed: ",
  forgotten: TR ? "Bu telefon artık eşli değil. Yeniden eşleştirin." : "This phone is not paired any more. Pair it again.",
  offline: TR ? "Bağlantı yok, yeniden deneniyor…" : "Offline, trying again…",
  watching: TR ? "Sadece izleme" : "Watching only",
  failed: TR ? "i-mux'a bağlanılamadı: " : "Could not reach i-mux: ",
};
for (const el of document.querySelectorAll("[data-t]")) el.innerHTML = T[el.dataset.t]; // our own text

// --- sealing ----------------------------------------------------------------
const enc = new TextEncoder();
const dec = new TextDecoder();
function b64uDecode(s) {
  s = s.replace(/-/g, "+").replace(/_/g, "/");
  while (s.length % 4) s += "=";
  return Uint8Array.from(atob(s), (c) => c.charCodeAt(0));
}
function b64Encode(b) {
  let s = "";
  for (let i = 0; i < b.length; i += 0x8000) s += String.fromCharCode.apply(null, b.subarray(i, i + 0x8000));
  return btoa(s);
}
const b64uEncode = (b) => b64Encode(b).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
const KEY = "imux-phone";
let me = null, key = null, aad = null;
try { me = JSON.parse(localStorage.getItem(KEY)); } catch {}
function useDevice(d) { me = d; key = b64uDecode(d.key); aad = enc.encode(d.id); }
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
  return gcm(key, b.subarray(0, 12), aad).decrypt(b.subarray(12));
}

// --- pairing ----------------------------------------------------------------
async function pairFromFragment() {
  const m = location.hash.match(/^#p=([A-Za-z0-9_-]+)\.([A-Za-z0-9_-]+)$/);
  if (!m) return;
  history.replaceState(null, "", location.pathname); // the key leaves the address bar
  const name = (/iPhone|iPad|Android/.exec(navigator.userAgent) || ["Phone"])[0];
  try {
    const r = await fetch("/api/pair", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token: m[1], name }) });
    if (!r.ok) throw new Error((await r.text()).trim());
    const { id } = await r.json();
    me = { id, key: m[2] };
    localStorage.setItem(KEY, JSON.stringify(me));
  } catch (e) {
    showPair(T.pair_failed + e.message);
  }
}
function showPair(err) {
  $("#pair").hidden = false;
  const p = $("#pair-error");
  p.hidden = !err;
  p.textContent = err || "";
}

// --- the terminal -------------------------------------------------------------
const stage = $("#stage");
const box = $("#term");
let fontFamily = "ui-monospace, Menlo, monospace";
let fontSize = Math.min(20, Math.max(6, parseFloat(localStorage.getItem("imux-fs")) || 9));
let readOnly = false;
let term = null;
let cell = { w: 6, h: 12 };

async function loadFont() {
  try {
    const f = new FontFace("IMuxNerd", "url(/font.ttf)");
    await f.load();
    document.fonts.add(f);
    fontFamily = '"IMuxNerd", ' + fontFamily;
  } catch { /* no Nerd Font served: the icons fall back */ }
}

function measure() {
  const c = document.createElement("canvas").getContext("2d");
  c.font = `${fontSize}px ${fontFamily}`;
  const w = c.measureText("W".repeat(20)).width / 20;
  cell = { w: Math.max(3, w), h: Math.ceil(fontSize * 1.18) };
  // xterm's own cell size, when it has drawn: the true one.
  const dims = term && term._core && term._core._renderService && term._core._renderService.dimensions;
  if (dims && dims.css && dims.css.cell && dims.css.cell.width) cell = { w: dims.css.cell.width, h: dims.css.cell.height };
}

function fitSize() {
  measure();
  return {
    cols: Math.max(20, Math.floor(stage.clientWidth / cell.w)),
    rows: Math.max(5, Math.floor(stage.clientHeight / cell.h)),
  };
}

function makeTerm() {
  term = new Terminal({
    fontFamily, fontSize, lineHeight: 1.0, cursorBlink: false, allowProposedApi: true,
    scrollback: 0, macOptionIsMeta: true, convertEol: false,
    theme: {
      background: "#000000", foreground: "#d0d7de", cursor: "#39c5cf", selectionBackground: "#39c5cf55",
      black: "#484f58", red: "#ff7b72", green: "#3fb950", yellow: "#d29922", blue: "#58a6ff", magenta: "#bc8cff", cyan: "#39c5cf", white: "#b1bac4",
      brightBlack: "#6e7681", brightRed: "#ffa198", brightGreen: "#56d364", brightYellow: "#e3b341", brightBlue: "#79c0ff", brightMagenta: "#d2a8ff", brightCyan: "#56d4dd", brightWhite: "#ffffff",
    },
  });
  term.open(box);
  term.onData((d) => typed(d));
  term.onBinary((d) => send(Uint8Array.from(d, (c) => c.charCodeAt(0) & 255)));
  keyboard(false);
}

// A watching phone draws the desktop's size, scaled to the screen.
function place() {
  if (!term) return;
  const el = term.element;
  const w = el.offsetWidth, h = el.offsetHeight;
  const scale = readOnly && w > 0 ? Math.min(1, stage.clientWidth / w, stage.clientHeight / h) : 1;
  box.style.transform = scale < 1 ? `scale(${scale})` : "";
}

// --- the stream -------------------------------------------------------------
let es = null, retry = null, connected = false;
function status(on) {
  connected = on;
  $("#dot").className = "dot " + (on ? "on" : "off");
}
function toast(msg, ms) {
  const t = $("#toast");
  t.textContent = msg;
  t.hidden = !msg;
  clearTimeout(toast.timer);
  if (msg && ms) toast.timer = setTimeout(() => (t.hidden = true), ms);
}

function connect() {
  clearTimeout(retry);
  if (es) es.close();
  const { cols, rows } = fitSize();
  term.resize(cols, rows);
  es = new EventSource("/api/term?d=" + encodeURIComponent(me.id) + "&a=" + encodeURIComponent(seal({ op: "term", cols, rows })));
  es.addEventListener("hello", (e) => {
    const h = JSON.parse(dec.decode(unseal(e.data)));
    readOnly = !!h.read_only;
    $("#app").classList.toggle("watching", readOnly);
    $("#name").textContent = readOnly ? "i-mux · " + T.watching : "i-mux";
    term.reset();
    term.resize(h.cols, h.rows);
    place();
    status(true);
    toast("");
  });
  es.onmessage = (e) => {
    try { term.write(unseal(e.data)); } catch {}
  };
  es.addEventListener("failed", (e) => {
    let msg = "";
    try { msg = JSON.parse(dec.decode(unseal(e.data))).error; } catch {}
    toast(T.failed + msg);
    es.close();
    status(false);
    retry = setTimeout(connect, 4000);
  });
  es.addEventListener("end", () => { es.close(); status(false); retry = setTimeout(connect, 1500); });
  es.onerror = async () => {
    es.close();
    status(false);
    try {
      const r = await fetch("/api/term?d=" + encodeURIComponent(me.id) + "&a=x");
      if (r.status === 401 && (await r.text()).includes("not paired")) {
        localStorage.removeItem(KEY);
        return showPair(T.forgotten);
      }
    } catch {}
    toast(T.offline);
    retry = setTimeout(connect, 2000);
  };
}

// Input goes out in order, one request at a time, in small batches.
let pending = [], sending = false;
function send(bytes) {
  if (readOnly || !connected) return;
  pending.push(bytes);
  if (!sending) flush();
}
async function flush() {
  sending = true;
  while (pending.length) {
    const n = pending.reduce((a, b) => a + b.length, 0);
    const all = new Uint8Array(n);
    let o = 0;
    for (const b of pending) { all.set(b, o); o += b.length; }
    pending = [];
    try {
      await fetch("/api/cmd", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ d: me.id, m: seal({ op: "input", data: b64Encode(all) }) }) });
    } catch {}
  }
  sending = false;
}
function sendText(s) { send(enc.encode(s)); }
async function resize() {
  if (!term) return;
  const { cols, rows } = fitSize();
  if (readOnly) { place(); return; }
  if (cols === term.cols && rows === term.rows) return;
  term.resize(cols, rows);
  if (connected) {
    try { await fetch("/api/cmd", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ d: me.id, m: seal({ op: "resize", cols, rows }) }) }); } catch {}
  }
}

// --- keys ---------------------------------------------------------------------
const mods = { ctrl: false, alt: false };
function setMod(m, on) {
  mods[m] = on;
  document.querySelector(`[data-mod="${m}"]`).classList.toggle("on", on);
}
// typed is what the keyboard gives, with a held Ctrl or Alt from the bar.
function typed(d) {
  if ((mods.ctrl || mods.alt) && d.length === 1) {
    let c = d;
    if (mods.ctrl) {
      const k = d.toLowerCase().charCodeAt(0);
      if (k >= 97 && k <= 122) c = String.fromCharCode(k - 96);
      else if (d === " ") c = "\x00";
      else if (d === "[") c = "\x1b";
    }
    if (mods.alt) c = "\x1b" + c;
    setMod("ctrl", false);
    setMod("alt", false);
    return sendText(c);
  }
  sendText(d);
}
function app() { return term && term.modes && term.modes.applicationCursorKeysMode; }
const SEQ = {
  esc: () => "\x1b", tab: () => "\t", enter: () => "\r", ctrlc: () => "\x03", prefix: () => "\x02",
  up: () => (app() ? "\x1bOA" : "\x1b[A"), down: () => (app() ? "\x1bOB" : "\x1b[B"),
  right: () => (app() ? "\x1bOC" : "\x1b[C"), left: () => (app() ? "\x1bOD" : "\x1b[D"),
  pgup: () => "\x1b[5~", pgdn: () => "\x1b[6~",
};
for (const b of document.querySelectorAll("#keys button")) {
  // pointerdown, not click: the keyboard stays open and nothing loses focus.
  b.addEventListener("pointerdown", (e) => {
    e.preventDefault();
    if (navigator.vibrate) navigator.vibrate(8);
    if (b.dataset.mod) return setMod(b.dataset.mod, !mods[b.dataset.mod]);
    let s = SEQ[b.dataset.seq]();
    if (mods.alt) s = "\x1b" + s;
    setMod("ctrl", false);
    setMod("alt", false);
    sendText(s);
  });
}

// The phone's keyboard opens only when asked: taps are the mouse.
let kbdOn = false;
function keyboard(on) {
  kbdOn = on;
  $("#kbd").classList.toggle("on", on);
  const ta = term && term.textarea;
  if (!ta) return;
  ta.setAttribute("inputmode", on ? "text" : "none");
  ta.setAttribute("autocapitalize", "off");
  ta.setAttribute("autocorrect", "off");
  ta.setAttribute("spellcheck", "false");
  if (on) ta.focus(); else ta.blur();
}
$("#kbd").addEventListener("pointerdown", (e) => { e.preventDefault(); keyboard(!kbdOn); });

function setFont(px) {
  fontSize = Math.min(20, Math.max(6, px));
  localStorage.setItem("imux-fs", String(fontSize));
  term.options.fontSize = fontSize;
  setTimeout(resize, 30);
}
$("#smaller").addEventListener("pointerdown", (e) => { e.preventDefault(); setFont(fontSize - 1); });
$("#bigger").addEventListener("pointerdown", (e) => { e.preventDefault(); setFont(fontSize + 1); });

// --- touch: a tap is a click (the browser's own), a long press a right-click,
// a swipe the mouse wheel ------------------------------------------------------
function cellAt(x, y) {
  const r = term.element.querySelector(".xterm-screen").getBoundingClientRect();
  const s = r.width / (term.cols * cell.w) || 1; // the watching scale
  return {
    col: Math.min(term.cols, Math.max(1, Math.floor((x - r.left) / (cell.w * s)) + 1)),
    row: Math.min(term.rows, Math.max(1, Math.floor((y - r.top) / (cell.h * s)) + 1)),
  };
}
function mouse(btn, x, y, release) {
  const { col, row } = cellAt(x, y);
  sendText(`\x1b[<${btn};${col};${row}${release ? "m" : "M"}`);
}
let touch = null;
stage.addEventListener("touchstart", (e) => {
  if (e.touches.length !== 1) return;
  const t = e.touches[0];
  touch = { x: t.clientX, y: t.clientY, acc: 0, moved: false, long: false };
  touch.timer = setTimeout(() => {
    if (!touch || touch.moved) return;
    touch.long = true;
    if (navigator.vibrate) navigator.vibrate(15);
    mouse(2, touch.x, touch.y, false);
    mouse(2, touch.x, touch.y, true);
  }, 500);
}, { passive: true });
stage.addEventListener("touchmove", (e) => {
  if (!touch || e.touches.length !== 1) return;
  const t = e.touches[0];
  const dy = t.clientY - touch.y;
  if (!touch.moved && Math.abs(dy) < 8 && Math.abs(t.clientX - touch.x) < 8) return;
  touch.moved = true;
  clearTimeout(touch.timer);
  e.preventDefault();
  touch.acc += dy;
  touch.y = t.clientY;
  const step = cell.h * 1.5;
  while (Math.abs(touch.acc) >= step) {
    const up = touch.acc > 0;
    touch.acc -= up ? step : -step;
    if (term.modes.mouseTrackingMode !== "none") {
      mouse(up ? 64 : 65, t.clientX, t.clientY, false);
    } else {
      term.scrollLines(up ? -1 : 1);
    }
  }
}, { passive: false });
stage.addEventListener("touchend", (e) => {
  if (!touch) return;
  clearTimeout(touch.timer);
  if (touch.moved || touch.long) e.preventDefault(); // no click after a swipe or a right-click
  touch = null;
  if (!kbdOn && term.textarea) term.textarea.blur();
}, { passive: false });

let resizeTimer = null;
window.addEventListener("resize", () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(resize, 150); });
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible" && me && key && !connected) connect();
});

// --- start --------------------------------------------------------------------
await pairFromFragment();
if (me && me.id && me.key) {
  useDevice(me);
  await loadFont();
  makeTerm();
  connect();
} else if ($("#pair").hidden) {
  showPair();
}
