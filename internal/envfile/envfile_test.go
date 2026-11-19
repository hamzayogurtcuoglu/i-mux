package envfile

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestParse(t *testing.T) {
	text := strings.Join([]string{
		"# a comment",
		"",
		"BASE_URL=https://example.test/api",
		"export TOKEN='fake key'",
		`QUOTED="say \"hi\" \\ there" # trailing comment`,
		"PLAIN=value # comment",
		"SPACED = padded ",
		"EMPTY=",
		"DUP=first",
		"DUP=second",
		"not a line",
		"1BAD=x",
		"PATH=/evil",
		"IMUX_PANE=%1",
		"OPEN='unclosed",
		"exported=lower",
		"\tINDENTED=yes\r",
	}, "\n")
	e := Parse("test", text)
	want := []Var{
		{"BASE_URL", "https://example.test/api"},
		{"TOKEN", "fake key"},
		{"QUOTED", `say "hi" \ there`},
		{"PLAIN", "value"},
		{"SPACED", "padded"},
		{"EMPTY", ""},
		{"DUP", "second"},
		{"exported", "lower"},
		{"INDENTED", "yes"},
	}
	if !reflect.DeepEqual(e.Vars, want) {
		t.Fatalf("vars\n got %q\nwant %q", e.Vars, want)
	}
	probs := []Problem{{11, BadLine}, {12, BadKey}, {13, ReservedKey}, {14, ReservedKey}, {15, BadQuote}}
	if !reflect.DeepEqual(e.Problems, probs) {
		t.Fatalf("problems %v", e.Problems)
	}
	if got := strings.Join(e.Keys(), " "); got != "BASE_URL TOKEN QUOTED PLAIN SPACED EMPTY DUP exported INDENTED" {
		t.Fatalf("keys %q", got)
	}
	if e.Map()["DUP"] != "second" {
		t.Fatal("map")
	}
}

func TestNames(t *testing.T) {
	for _, n := range []string{"glm", "z-ai", "open_router", "a1", strings.Repeat("a", 32)} {
		if !ValidName(n) {
			t.Errorf("%q should be valid", n)
		}
	}
	for _, n := range []string{"", "GLM", "-x", "_x", "a b", "a/b", "../x", ".apply-1", strings.Repeat("a", 33)} {
		if ValidName(n) {
			t.Errorf("%q should not be valid", n)
		}
	}
}

// TestSlug: what is typed in the New environment dialog becomes a valid
// name, or "".
func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"glm":                      "glm",
		"GLM Coding":               "glm-coding",
		"  Z.ai / GLM  ":           "z-ai-glm",
		"İş_proxy":                 "is_proxy",
		"Çalışma Ortamı":           "calisma-ortami",
		"Équipe":                   "equipe",
		"_x":                       "x",
		"a--b":                     "a-b",
		"日本":                       "",
		"":                         "",
		strings.Repeat("a", 40):    strings.Repeat("a", 32),
		strings.Repeat("abc-", 10): strings.TrimRight(strings.Repeat("abc-", 8), "-"),
		strings.Repeat("ab_", 11):  strings.Repeat("ab_", 10) + "ab",
	} {
		got := Slug(in)
		if got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
		if got != "" && !ValidName(got) {
			t.Errorf("Slug(%q) = %q is not a valid name", in, got)
		}
	}
}

// TestTemplateExamples: the template is all comments, and its examples,
// uncommented, set what they show (a trailing comment is not a value).
func TestTemplateExamples(t *testing.T) {
	defer i18n.SetLocale("en")
	example := regexp.MustCompile(`(?m)^# ([A-Z_]+=)`)
	for _, l := range []string{"en", "tr"} {
		i18n.SetLocale(l)
		text := Template("glm")
		if e := Parse("glm", text); len(e.Vars) != 0 || len(e.Problems) != 0 {
			t.Fatalf("%s: the template sets nothing: %+v", l, e)
		}
		e := Parse("glm", example.ReplaceAllString(text, "$1"))
		m := e.Map()
		if len(e.Problems) != 0 || m["HTTPS_PROXY"] != "http://proxy.example.com:8080" ||
			m["NO_PROXY"] != "localhost,127.0.0.1" || m["ANTHROPIC_BASE_URL"] != "https://openrouter.ai/api" ||
			strings.ContainsAny(m["GITHUB_TOKEN"], " #") || m["GITHUB_TOKEN"] == "" {
			t.Fatalf("%s: examples %+v", l, e)
		}
	}
}

// Files: made with the template (0600, in a 0700 folder), listed by name
// (apply files and other names left out), read, deleted.
func TestFiles(t *testing.T) {
	testutil.Isolate(t)
	i18n.SetLocale("en")
	if List() != nil {
		t.Fatal("no folder: no environments")
	}
	p, err := Create("glm")
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %v %v", fi, err)
	}
	if di, err := os.Stat(Dir()); err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v %v", di, err)
	}
	e, err := Read("glm")
	if err != nil || len(e.Vars) != 0 || len(e.Problems) != 0 {
		t.Fatalf("the template is all comments: %+v %v", e, err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "# ANTHROPIC_BASE_URL=") {
		t.Fatalf("template %q", b)
	}
	testutil.WriteFile(t, p, []byte("A=1\nB=2\n"))
	if _, err := Create("glm"); err != nil {
		t.Fatal(err)
	}
	if e, _ := Read("glm"); len(e.Vars) != 2 {
		t.Fatal("Create must keep an existing environment")
	}
	testutil.WriteFile(t, Path("alpha"), []byte("X=1\n"))
	testutil.WriteFile(t, filepath.Join(Dir(), ".apply-00"), []byte("x"))
	testutil.WriteFile(t, filepath.Join(Dir(), "Bad Name.env"), []byte("x"))
	testutil.WriteFile(t, filepath.Join(Dir(), "notes.txt"), []byte("x"))
	var names []string
	for _, e := range List() {
		names = append(names, e.Name)
	}
	if strings.Join(names, " ") != "alpha glm" {
		t.Fatalf("list %v", names)
	}
	if !Exists("alpha") || Exists("nope") || Exists("../x") {
		t.Fatal("Exists")
	}
	if err := Delete("alpha"); err != nil || Exists("alpha") {
		t.Fatalf("delete %v", err)
	}
	if _, err := Create("Bad"); err != ErrName {
		t.Fatalf("bad name: %v", err)
	}
	if _, err := Read("../config"); err != ErrName {
		t.Fatalf("read bad name: %v", err)
	}
}

func TestScript(t *testing.T) {
	e := Parse("x", "A=1\nB=it's\n")
	got := Script(e, []string{"A", "OLD", "PATH", "bad key"}, false, "/h/.imux/env/.apply-1")
	want := "unset OLD\nexport A='1'\nexport B='it'\\''s'\ncommand rm -f -- '/h/.imux/env/.apply-1'\n"
	if got != want {
		t.Fatalf("posix\n got %q\nwant %q", got, want)
	}
	got = Script(e, []string{"OLD"}, true, "/f")
	want = "set -e OLD\nset -gx A '1'\nset -gx B 'it\\'s'\ncommand rm -f -- '/f'\n"
	if got != want {
		t.Fatalf("fish\n got %q\nwant %q", got, want)
	}
	if got := Script(nil, []string{"A"}, false, "/f"); got != "unset A\ncommand rm -f -- '/f'\n" {
		t.Fatalf("clear %q", got)
	}
}

// The apply file in a real POSIX shell: the values come back as they were,
// the variables left behind are unset, and the file deletes itself.
func TestScriptInShell(t *testing.T) {
	testutil.Isolate(t)
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	value := `it's "quoted" $HOME \n ` + "`x`; two\nlines"
	e := &Env{Vars: []Var{{"ENVTEST_A", value}, {"ENVTEST_B", "b"}}}
	p, err := WriteScript(e, []string{"ENVTEST_OLD"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("apply file %v %v", fi, err)
	}
	cmd := exec.Command(sh, "-c", `. "$1"; printf '%s|%s|%s' "$ENVTEST_A" "$ENVTEST_B" "${ENVTEST_OLD-unset}"`, "sh", p)
	cmd.Env = append(os.Environ(), "ENVTEST_OLD=old")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != value+"|b|unset" {
		t.Fatalf("got %q", out)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("the apply file must delete itself")
	}
	if fish, err := exec.LookPath("fish"); err == nil {
		p, _ := WriteScript(e, nil, true)
		out, err := exec.Command(fish, "-c", `source $argv[1]; printf '%s|%s' "$ENVTEST_A" "$ENVTEST_B"`, p).Output()
		if err != nil || string(out) != value+"|b" {
			t.Fatalf("fish: %q %v", out, err)
		}
	}
}

func TestSweepScripts(t *testing.T) {
	testutil.Isolate(t)
	p, err := WriteScript(nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	keep := Path("glm")
	testutil.WriteFile(t, keep, []byte("A=1\n"))
	SweepScripts(0)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("an old apply file goes")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("environments stay")
	}
}

func FuzzParse(f *testing.F) {
	f.Add("A=1\nexport B='x'\n# c\nC=\"q\\\"\"\n")
	f.Add("=\n'\n\"\nexport\nexport =1")
	f.Fuzz(func(t *testing.T, s string) {
		e := Parse("f", s)
		seen := map[string]bool{}
		for _, v := range e.Vars {
			if !keyRe.MatchString(v.Key) || Reserved(v.Key) || seen[v.Key] || strings.IndexByte(v.Value, 0) >= 0 {
				t.Fatalf("bad var %q", v)
			}
			seen[v.Key] = true
		}
	})
}
