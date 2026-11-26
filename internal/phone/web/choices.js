// Quick answers (D110): the numbered choices an agent lists at the bottom
// of its screen ("1. Yes", "❯ 2. No, and tell Copilot…"), as the phone
// shows them for one tap. A separate module so it can be tested alone.

const ESC = /\x1b\[[0-9;:?]*[A-Za-z]|\x1b[()][A-Za-z0-9]|\x1b./g;
const LINE = /^[\s│┃|]*(?:([❯›>▶➤●◉→])\s*)?([1-9])[.)]\s+(.*?)[\s│┃|]*$/u;
// What may follow the choices: blank lines, a box's border, a hint about keys.
const AFTER = /^[\s│┃|╰╯╭╮─━┄┈═]*$|enter|esc|navigate|select|cancel|confirm|↑|↓|tab to|ctrl/i;

// choices is the numbered lines (1, 2, 3… in order) at the bottom of the
// screen, followed only by blank lines, borders or key hints:
// [{n, label, current}], or [] when the screen ends otherwise.
export function choices(text, bottom = 30) {
  const tail = String(text || "").replace(ESC, "").split("\n").slice(-bottom);
  let run = [], end = -1;
  tail.forEach((l, i) => {
    const m = LINE.exec(l);
    const label = m ? m[3].trim() : "";
    if (!m || !label) {
      // A wrapped label's rest, right below its choice.
      const last = run[run.length - 1];
      const rest = l.replace(/[\s│┃|]+$/u, "").match(/^([\s│┃|]+)(\S.*)$/u);
      if (rest && last && end === i - 1 && rest[1].length >= last.col) { last.label += " " + rest[2].trim(); end = i; }
      return;
    }
    const c = { n: +m[2], label, current: !!m[1], col: l.indexOf(m[3]) };
    if (c.n === 1) run = [c];
    else if (run.length && c.n === run[run.length - 1].n + 1 && end === i - gap(tail, end, i)) run.push(c);
    else return void (run = []);
    end = i;
  });
  if (run.length < 2 || end < 0) return [];
  for (const l of tail.slice(end + 1)) if (!AFTER.test(l)) return [];
  return run.map(({ n, label, current }) => ({ n, label, current }));
}

// gap is how many lines between the last choice (at end) and line i are a
// wrapped label's rest or blank: they belong to the choices.
function gap(lines, end, i) {
  for (let k = end + 1; k < i; k++) if (lines[k].trim() !== "" && !/^[\s│┃|]{3,}\S/.test(lines[k])) return -1;
  return i - end;
}
