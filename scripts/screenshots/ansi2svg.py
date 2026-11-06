#!/usr/bin/env python3
"""Render a `tmux capture-pane -p -e` screen as an SVG terminal window.

Usage: ansi2svg.py COLS ROWS TITLE < capture > out.svg

Handles the SGR attributes tmux emits (bold, dim, italic, underline, reverse,
16/256/RGB colors) and double-width characters. Each run of same-styled text
is placed on its exact cell, so columns line up in any monospace font.
"""
import re
import sys
import unicodedata
from html import escape

CELL_W, CELL_H, FONT = 8.4, 18.0, 14
PAD, BAR = 16, 34
BG, FG = "#0d1117", "#d0d7de"
PALETTE = [
    "#484f58", "#ff7b72", "#3fb950", "#d29922", "#58a6ff", "#bc8cff", "#39c5cf", "#b1bac4",
    "#6e7681", "#ffa198", "#56d364", "#e3b341", "#79c0ff", "#d2a8ff", "#56d4dd", "#ffffff",
]


def xterm256(n):
    if n < 16:
        return PALETTE[n]
    if n < 232:
        n -= 16
        lv = [0, 95, 135, 175, 215, 255]
        return "#%02x%02x%02x" % (lv[n // 36], lv[n // 6 % 6], lv[n % 6])
    g = 8 + (n - 232) * 10
    return "#%02x%02x%02x" % (g, g, g)


def width(ch):
    if unicodedata.combining(ch):
        return 0
    return 2 if unicodedata.east_asian_width(ch) in ("W", "F") else 1


class Style:
    def __init__(self):
        self.reset()

    def reset(self):
        self.fg = self.bg = None
        self.bold = self.dim = self.italic = self.under = self.rev = False

    def key(self):
        return (self.fg, self.bg, self.bold, self.dim, self.italic, self.under, self.rev)

    def apply(self, params):
        p = [int(x) if x else 0 for x in params.split(";")] if params else [0]
        i = 0
        while i < len(p):
            c = p[i]
            if c == 0:
                self.reset()
            elif c == 1:
                self.bold = True
            elif c == 2:
                self.dim = True
            elif c == 3:
                self.italic = True
            elif c == 4:
                self.under = True
            elif c == 7:
                self.rev = True
            elif c == 22:
                self.bold = self.dim = False
            elif c == 23:
                self.italic = False
            elif c == 24:
                self.under = False
            elif c == 27:
                self.rev = False
            elif 30 <= c <= 37:
                self.fg = PALETTE[c - 30]
            elif 90 <= c <= 97:
                self.fg = PALETTE[c - 90 + 8]
            elif 40 <= c <= 47:
                self.bg = PALETTE[c - 40]
            elif 100 <= c <= 107:
                self.bg = PALETTE[c - 100 + 8]
            elif c == 39:
                self.fg = None
            elif c == 49:
                self.bg = None
            elif c in (38, 48) and i + 1 < len(p):
                if p[i + 1] == 5 and i + 2 < len(p):
                    col = xterm256(p[i + 2])
                    i += 2
                elif p[i + 1] == 2 and i + 4 < len(p):
                    col = "#%02x%02x%02x" % tuple(p[i + 2:i + 5])
                    i += 4
                else:
                    col = None
                if c == 38:
                    self.fg = col
                else:
                    self.bg = col
            i += 1


TOKEN = re.compile(r"\x1b\[([0-9;]*)m|\x1b\[[0-9;?]*[A-Za-z]|\x1b[()][A-Za-z0-9]|(.)", re.S)


def parse(text, cols, rows):
    """Returns rows of runs: (col, ncells, text, style key)."""
    st = Style()
    out = []
    for line in text.split("\n")[:rows]:
        runs, col = [], 0
        for m in TOKEN.finditer(line):
            if m.group(1) is not None:
                st.apply(m.group(1))
                continue
            ch = m.group(2)
            if ch is None or ch == "\r":
                continue
            w = width(ch)
            if w == 0 or col + w > cols:
                continue
            k = st.key()
            if runs and runs[-1][3] == k and runs[-1][0] + runs[-1][1] == col:
                c0, n, s, _ = runs[-1]
                runs[-1] = (c0, n + w, s + ch, k)
            else:
                runs.append((col, w, ch, k))
            col += w
        out.append(runs)
    while len(out) < rows:
        out.append([])
    return out


def colors(k):
    fg, bg, bold, dim, *_rest = k
    rev = k[6]
    fg, bg = fg or FG, bg
    if rev:
        fg, bg = (bg or BG), fg
    return fg, bg


def render(rows, cols, nrows, title):
    w = PAD * 2 + cols * CELL_W
    h = BAR + PAD + nrows * CELL_H + PAD
    o = [
        f'<svg xmlns="http://www.w3.org/2000/svg" xml:space="preserve" width="{w:.0f}" height="{h:.0f}" viewBox="0 0 {w:.0f} {h:.0f}">',
        '<style>text{font-family:ui-monospace,SFMono-Regular,"SF Mono",Menlo,Consolas,"DejaVu Sans Mono",'
        f'"Liberation Mono",monospace;font-size:{FONT}px;white-space:pre}}</style>',
        f'<rect width="{w:.0f}" height="{h:.0f}" rx="10" fill="{BG}"/>',
        f'<rect width="{w:.0f}" height="{BAR}" rx="10" fill="#161b22"/>',
        f'<rect y="{BAR - 10}" width="{w:.0f}" height="10" fill="#161b22"/>',
    ]
    for i, c in enumerate(("#ff5f57", "#febc2e", "#28c840")):
        o.append(f'<circle cx="{20 + i * 20}" cy="{BAR / 2}" r="6" fill="{c}"/>')
    o.append(f'<text x="{w / 2:.0f}" y="{BAR / 2 + 5}" fill="#8b949e" text-anchor="middle">{escape(title)}</text>')
    y0 = BAR + PAD
    for r, runs in enumerate(rows):
        y = y0 + r * CELL_H
        for col, n, s, k in runs:
            _, bg = colors(k)
            if bg:
                o.append(f'<rect x="{PAD + col * CELL_W:.1f}" y="{y:.1f}" width="{n * CELL_W:.1f}" height="{CELL_H}" fill="{bg}"/>')
    for r, runs in enumerate(rows):
        y = y0 + r * CELL_H + CELL_H * 0.75
        for col, n, s, k in runs:
            body = s.strip(" ")
            if not body:
                continue
            col += len(s) - len(s.lstrip(" "))
            # One x per character pins every glyph to its cell, whatever the
            # font's advance width (textLength is not honored everywhere).
            xs = []
            for ch in body:
                xs.append(f"{PAD + col * CELL_W:.1f}".rstrip("0").rstrip("."))
                col += width(ch)
            fg, _ = colors(k)
            attrs = [f'x="{" ".join(xs)}"', f'y="{y:.1f}"', f'fill="{fg}"']
            if k[2]:
                attrs.append('font-weight="bold"')
            if k[3]:
                attrs.append('opacity="0.6"')
            if k[4]:
                attrs.append('font-style="italic"')
            if k[5]:
                attrs.append('text-decoration="underline"')
            o.append(f"<text {' '.join(attrs)}>{escape(body)}</text>")
    o.append("</svg>")
    return "\n".join(o) + "\n"


def main():
    cols, rows, title = int(sys.argv[1]), int(sys.argv[2]), sys.argv[3]
    text = sys.stdin.read()
    sys.stdout.write(render(parse(text, cols, rows), cols, rows, title))


if __name__ == "__main__":
    main()
