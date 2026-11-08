package gotofile

import (
	"reflect"
	"testing"
)

var files = []string{
	"README.md",
	"internal/sidebar/render.go",
	"internal/sidebar/render_test.go",
	"internal/sidebar/run.go",
	"internal/search/search.go",
	"internal/searchview/searchview.go",
	"internal/workspace/settings.go",
	"docs/CONFIG.md",
	"e2e/explorer_menu_test.go",
	"internal/config/default.toml",
}

func paths(rs []Result) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Path)
	}
	return out
}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Query
	}{
		{"render", Query{Parts: []string{"render"}}},
		{"  side rend ", Query{Parts: []string{"side", "rend"}}},
		{"Render", Query{Parts: []string{"Render"}, Exact: true}},
		{"run.go:42", Query{Parts: []string{"run.go"}, Line: 42}},
		{"run.go:", Query{Parts: []string{"run.go"}}},
		{"a:b", Query{Parts: []string{"a:b"}}},
		{"", Query{}},
	}
	for _, c := range cases {
		if got := Parse(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestRank(t *testing.T) {
	cases := []struct {
		q     string
		first string
	}{
		{"render", "internal/sidebar/render.go"},          // the name, shorter than render_test.go
		{"rendertest", "internal/sidebar/render_test.go"}, // letters in order, not together
		{"sbrnd", "internal/sidebar/render.go"},           // across folder and name
		{"run", "internal/sidebar/run.go"},                // the name over folders with r-u-n
		{"searchview", "internal/searchview/searchview.go"},
		{"config", "docs/CONFIG.md"},         // case-insensitive
		{"emt", "e2e/explorer_menu_test.go"}, // word starts
		{"set", "internal/workspace/settings.go"},
		{"default toml", "internal/config/default.toml"}, // parts
	}
	for _, c := range cases {
		got := Rank(files, Parse(c.q), 50)
		if len(got) == 0 || got[0].Path != c.first {
			t.Errorf("%q: %v", c.q, paths(got))
		}
	}
	// No match: letters out of order, or a capital that is not there.
	if got := Rank(files, Parse("gor"), 50); len(got) != 0 {
		t.Errorf("gor: %v", paths(got))
	}
	if got := Rank(files, Parse("Render"), 50); len(got) != 0 {
		t.Errorf("Render (case-sensitive): %v", paths(got))
	}
	if got := Rank(files, Parse("README"), 50); len(got) != 1 {
		t.Errorf("README: %v", paths(got))
	}
	if got := Rank(files, Parse("go"), 3); len(got) != 3 {
		t.Errorf("limit: %v", paths(got))
	}
}

func TestMatchPositions(t *testing.T) {
	_, pos, ok := Match("internal/sidebar/run.go", Parse("run"))
	if !ok || !reflect.DeepEqual(pos, []int{17, 18, 19}) {
		t.Fatalf("run: %v %v", pos, ok)
	}
	// Two parts: both highlighted, once each.
	_, pos, ok = Match("docs/CONFIG.md", Parse("doc md"))
	if !ok || !reflect.DeepEqual(pos, []int{0, 1, 2, 12, 13}) {
		t.Fatalf("doc md: %v %v", pos, ok)
	}
	// Unicode names.
	if _, pos, ok := Match("notlar/Masaüstü.txt", Parse("üst")); !ok || !reflect.DeepEqual(pos, []int{11, 12, 13}) {
		t.Fatalf("unicode: %v %v", pos, ok)
	}
}

func BenchmarkRank(b *testing.B) {
	var many []string
	for i := 0; i < 2000; i++ {
		for _, f := range files {
			many = append(many, "pkg"+string(rune('a'+i%26))+"/"+f)
		}
	}
	q := Parse("sbrndtest")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Rank(many, q, 200)
	}
}
