package sidebar

import (
	"path/filepath"
	"strings"
)

// Colors of the explorer's file icons: the palette of the editor's default
// file icon theme (Seti) in 256 colors.
const (
	icBlue   = "\x1b[38;5;74m"
	icCyan   = "\x1b[38;5;38m" // Go's own
	icYellow = "\x1b[38;5;185m"
	icOrange = "\x1b[38;5;173m"
	icRed    = "\x1b[38;5;167m"
	icGreen  = "\x1b[38;5;113m"
	icPurple = "\x1b[38;5;140m"
	icPink   = "\x1b[38;5;204m"
	icGrey   = "\x1b[38;5;245m"
	icWhite  = "\x1b[38;5;252m"
	icFolder = "\x1b[38;5;110m"
	icAgent  = "\x1b[38;5;141m" // agent instructions and skills
)

// fileIcon marks a kind of file in the explorer (D35): a Nerd Font glyph
// (Seti's, as the editor shows files, or a codicon) in its color, and an
// emoji for the emoji theme. Glyphs take one column, emoji two.
type fileIcon struct{ glyph, color, emoji string }

// Nerd Font glyphs (v3) used by more than one kind.
const (
	gFile     = "\ue64e" // seti default
	gGo       = "\ue627" // seti go
	gMarkdown = "\ue609"
	gJSON     = "\ue60b"
	gYAML     = "\ue6a8"
	gConfig   = "\ue615"
	gLock     = "\ue672"
	gShell    = "\ue691"
	gReact    = "\ue625"
	gCSS      = "\ue614"
	gC        = "\ue649"
	gCpp      = "\ue646"
	gJava     = "\ue66d"
	gDB       = "\ue64d"
	gImage    = "\ue60d"
	gZip      = "\ue6aa"
	gKey      = "\ueb11" // codicon key
	gDocker   = "\ue650"
	gMake     = "\ue673"
	gPython   = "\ue606"
	gRuby     = "\ue605"
	gRust     = "\ue68b"
	gGit      = "\ue65d"
	gBeaker   = "\uea79" // codicon beaker: tests
)

var (
	iconFile       = fileIcon{gFile, icWhite, "📄"}
	iconFolder     = fileIcon{"\ue5ff", icFolder, "📁"}
	iconFolderOpen = fileIcon{"\ue5fe", icFolder, "📂"}
	iconReadme     = fileIcon{"\ue66a", icBlue, "📖"}
	iconLicense    = fileIcon{"\ue60a", icYellow, "📜"}
)

// fileNames are the icons of whole names (lowercase): manifests, lock
// files, the agents' instructions, skills ...
var fileNames = map[string]fileIcon{}

// fileExts are the icons by extension (lowercase, with its dot).
var fileExts = map[string]fileIcon{}

func init() {
	add := func(m map[string]fileIcon, ic fileIcon, keys ...string) {
		for _, k := range keys {
			m[k] = ic
		}
	}
	n := func(ic fileIcon, names ...string) { add(fileNames, ic, names...) }
	e := func(ic fileIcon, exts ...string) { add(fileExts, ic, exts...) }

	// i-mux's own: what the agents read (the rail's Agents and Skills icons).
	n(fileIcon{"\ueb08", icAgent, "🤖"}, "agents.md", "agent.md", "claude.md", "gemini.md", "copilot-instructions.md")
	n(fileIcon{"\ueae6", icAgent, "🧩"}, "skill.md")
	n(fileIcon{gGo, icBlue, "📦"}, "go.mod", "go.work")
	n(fileIcon{gLock, icGrey, "🔒"}, "go.sum", "go.work.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		"cargo.lock", "gemfile.lock", "poetry.lock", "composer.lock", "uv.lock", "bun.lock", "bun.lockb", "flake.lock")
	n(fileIcon{gMake, icOrange, "🔨"}, "makefile", "gnumakefile", "cmakelists.txt", "justfile")
	n(fileIcon{gDocker, icBlue, "🐳"}, "dockerfile", "containerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml")
	n(fileIcon{"\ue616", icRed, "📦"}, "package.json")
	n(fileIcon{gRust, icOrange, "📦"}, "cargo.toml")
	n(fileIcon{gRuby, icRed, "💎"}, "gemfile", "rakefile")
	n(fileIcon{gPython, icBlue, "🐍"}, "requirements.txt", "pyproject.toml", "pipfile", "setup.py", "setup.cfg")
	n(fileIcon{"\ue641", icGreen, "🕒"}, "changelog.md", "changelog", "changes.md", "history.md")
	n(fileIcon{"\ue69c", icYellow, "✅"}, "tasks.md", "todo.md", "todo")
	n(fileIcon{"\ue65b", icWhite, "👥"}, "codeowners")
	n(fileIcon{gGit, icOrange, "🔧"}, ".gitignore", ".gitattributes", ".gitmodules", ".gitkeep")
	n(fileIcon{gKey, icYellow, "🔑"}, ".env", ".envrc", ".env.local", ".env.example")
	n(fileIcon{gConfig, icGrey, "🔧"}, ".editorconfig", ".prettierrc", ".eslintrc", ".npmrc")

	e(fileIcon{gGo, icCyan, "🐹"}, ".go")
	e(fileIcon{gMarkdown, icBlue, "📝"}, ".md", ".markdown", ".mdx", ".rst", ".adoc", ".org")
	e(iconFile, ".txt", ".text")
	e(fileIcon{gFile, icGrey, "🧾"}, ".log")
	e(fileIcon{gJSON, icYellow, "📋"}, ".json", ".jsonc", ".json5")
	e(fileIcon{gYAML, icPurple, "📋"}, ".yml", ".yaml")
	e(fileIcon{"\ue6b2", icOrange, "🔧"}, ".toml")
	e(fileIcon{gConfig, icGrey, "🔧"}, ".ini", ".cfg", ".conf", ".properties", ".config")
	e(fileIcon{"\ue619", icOrange, "📋"}, ".xml", ".plist")
	e(fileIcon{"\ue64a", icGreen, "📋"}, ".csv", ".tsv")
	e(fileIcon{"\ue60c", icYellow, "🟨"}, ".js", ".mjs", ".cjs")
	e(fileIcon{gReact, icBlue, "🟨"}, ".jsx")
	e(fileIcon{"\ue628", icBlue, "🟦"}, ".ts", ".mts", ".cts")
	e(fileIcon{gReact, icBlue, "🟦"}, ".tsx")
	e(fileIcon{gPython, icBlue, "🐍"}, ".py", ".pyi", ".pyw")
	e(fileIcon{gPython, icOrange, "📓"}, ".ipynb")
	e(fileIcon{gShell, icGreen, "🐚"}, ".sh", ".bash", ".zsh", ".fish", ".ksh")
	e(fileIcon{"\ue683", icBlue, "🐚"}, ".ps1", ".psm1", ".psd1")
	e(fileIcon{"\ue62a", icBlue, "🐚"}, ".bat", ".cmd")
	e(fileIcon{"\ue60e", icOrange, "🌐"}, ".html", ".htm", ".xhtml")
	e(fileIcon{gCSS, icBlue, "🎨"}, ".css", ".less")
	e(fileIcon{"\ue603", icPink, "🎨"}, ".scss", ".sass")
	e(fileIcon{gRust, icOrange, "🦀"}, ".rs")
	e(fileIcon{gJava, icRed, "☕"}, ".java", ".class", ".jar")
	e(fileIcon{"\ue634", icOrange, "☕"}, ".kt", ".kts")
	e(fileIcon{"\ue68e", icRed, "🟥"}, ".scala", ".sc")
	e(fileIcon{gRuby, icRed, "💎"}, ".rb", ".erb")
	e(fileIcon{gC, icBlue, "🔷"}, ".c", ".h")
	e(fileIcon{gCpp, icBlue, "🔷"}, ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx")
	e(fileIcon{"\ue648", icBlue, "🔷"}, ".cs")
	e(fileIcon{"\ue699", icOrange, "🐦"}, ".swift")
	e(fileIcon{"\ue608", icPurple, "🐘"}, ".php")
	e(fileIcon{"\ue620", icBlue, "🌙"}, ".lua")
	e(fileIcon{"\ue68a", icBlue, "📈"}, ".r")
	e(fileIcon{"\ue64c", icBlue, "🎯"}, ".dart")
	e(fileIcon{"\ue62d", icPurple, "💧"}, ".ex", ".exs")
	e(fileIcon{"\ue61f", icPurple, "🟪"}, ".hs", ".lhs")
	e(fileIcon{"\ue67a", icOrange, "🐫"}, ".ml", ".mli")
	e(fileIcon{"\ue67e", icBlue, "🐪"}, ".pl", ".pm")
	e(fileIcon{"\ue624", icPurple, "🟣"}, ".jl")
	e(fileIcon{"\ue677", icYellow, "👑"}, ".nim")
	e(fileIcon{"\ue6a9", icOrange, "⚡"}, ".zig")
	e(fileIcon{"\ue6a0", icGreen, "🟩"}, ".vue")
	e(fileIcon{"\ue697", icOrange, "🟧"}, ".svelte")
	e(fileIcon{"\ue662", icPink, "🟪"}, ".graphql", ".gql")
	e(fileIcon{"\ue69a", icPurple, "🟪"}, ".tf", ".tfvars", ".hcl")
	e(fileIcon{"\ue6a1", icPurple, "🟪"}, ".wasm")
	e(fileIcon{gDB, icPink, "💾"}, ".sql", ".db", ".sqlite", ".sqlite3")
	e(fileIcon{"\ue62b", icGreen, "📝"}, ".vim")
	e(fileIcon{"\ue69b", icGreen, "📝"}, ".tex", ".bib")
	e(fileIcon{"\ue66f", icRed, "📄"}, ".j2", ".jinja", ".jinja2")
	e(fileIcon{gLock, icGrey, "🔒"}, ".lock")
	e(fileIcon{gZip, icGrey, "📦"}, ".zip", ".tar", ".gz", ".tgz", ".xz", ".bz2", ".7z", ".rar", ".zst")
	e(fileIcon{"\ue67d", icRed, "📕"}, ".pdf")
	e(fileIcon{gImage, icPurple, "📷"}, ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico", ".tiff", ".avif", ".heic")
	e(fileIcon{"\ue698", icYellow, "📷"}, ".svg")
	e(fileIcon{"\ue69f", icPink, "🎬"}, ".mp4", ".mov", ".webm", ".mkv", ".avi")
	e(fileIcon{"\ue638", icPink, "🎵"}, ".mp3", ".wav", ".ogg", ".flac", ".m4a")
	e(fileIcon{"\ue659", icRed, "🔤"}, ".ttf", ".otf", ".woff", ".woff2")
	e(fileIcon{gGit, icOrange, "📄"}, ".diff", ".patch")
	e(fileIcon{gKey, icYellow, "🔑"}, ".env", ".pem", ".key", ".crt", ".cer", ".p12")
	e(fileIcon{"\ueaf8", icGrey, "🔩"}, ".exe", ".dll", ".so", ".dylib", ".bin", ".o", ".a", ".out")
	e(fileIcon{gMake, icOrange, "🔨"}, ".mk")
	e(fileIcon{gDocker, icBlue, "🐳"}, ".dockerfile")
}

// iconOf is the icon of an explorer entry: a folder's, open or closed; a
// file's by its whole name, then README / LICENSE, a test, its extension.
func iconOf(name string, dir, open bool) fileIcon {
	if dir {
		if open {
			return iconFolderOpen
		}
		return iconFolder
	}
	low := strings.ToLower(name)
	if ic, ok := fileNames[low]; ok {
		return ic
	}
	switch {
	case strings.HasPrefix(low, "readme"):
		return iconReadme
	case strings.HasPrefix(low, "license"), strings.HasPrefix(low, "licence"), strings.HasPrefix(low, "copying"): //nolint:misspell // LICENCE: the British file name
		return iconLicense
	}
	ic, ok := fileExts[filepath.Ext(low)]
	if !ok {
		ic = iconFile
	}
	if isTest(low) {
		ic.glyph, ic.emoji = gBeaker, "🧪"
	}
	return ic
}

// isTest reports a test file's name: x_test.go, x.test.ts, x.spec.js,
// test_x.py, x_test.py, x_spec.rb.
func isTest(low string) bool {
	ext := filepath.Ext(low)
	base := strings.TrimSuffix(low, ext)
	switch {
	case ext == ".go" || ext == ".py" || ext == ".rs" || ext == ".rb" || ext == ".exs":
		return strings.HasSuffix(base, "_test") || strings.HasSuffix(base, "_spec") || ext == ".py" && strings.HasPrefix(base, "test_")
	case strings.HasSuffix(base, ".test"), strings.HasSuffix(base, ".spec"):
		return true
	}
	return false
}

// draw is the icon in theme and its width: a colored glyph (one column) for
// the editor's icons, else an emoji (two).
func (ic fileIcon) draw(theme string) (string, int) {
	if theme == IconsNerd {
		return ic.color + ic.glyph + reset, 1
	}
	return ic.emoji, 2
}

// FileIcon is file name's icon as the explorer draws it in theme, and its
// width (the search popup's files).
func FileIcon(name, theme string) (string, int) { return iconOf(name, false, false).draw(theme) }
