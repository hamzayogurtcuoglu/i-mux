package jsonc

import (
	"encoding/json"
	"strings"
	"testing"
)

// wt is a Windows Terminal settings.json as it writes one.
const wt = `{
    "$schema": "https://aka.ms/terminal-profiles-schema",
    "defaultProfile": "{61c54bbd-c2c6-5271-96e7-009a87ff44bf}",
    "profiles": 
    {
        "defaults": {},
        "list": 
        [
            {
                "guid": "{17bf3de4-5353-5709-bcf9-835bd952a95e}",
                "name": "Ubuntu-22.04"
            }
        ]
    }
}
`

// vscode is a VS Code user settings.json: comments and a trailing comma.
const vscode = "\ufeff{\n" +
	"    // my editor\n" +
	"    \"editor.fontSize\": 14, /* big */\n" +
	"    \"a.b\": {\"x\": [1, 2, {\"y\": \"//not a comment\"}],},\n" +
	"}\n"

func valid(t *testing.T, b []byte) {
	t.Helper()
	var v any
	if err := json.Unmarshal(clean(b), &v); err != nil {
		t.Fatalf("not JSON(C) any more: %v\n%s", err, b)
	}
}

// TestSetWindowsTerminal: a font in the empty profiles.defaults is written
// as an indented block; the rest of the file is untouched; then replaced.
func TestSetWindowsTerminal(t *testing.T) {
	out, err := Set([]byte(wt), "JetBrainsMono NF", "profiles", "defaults", "font", "face")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(wt, `"defaults": {},`, "\"defaults\": {\n            \"font\": {\"face\": \"JetBrainsMono NF\"}\n        },", 1)
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
	valid(t, out)
	if v, ok := Get(out, "profiles", "defaults", "font", "face"); !ok || v != "JetBrainsMono NF" {
		t.Fatalf("Get = %q %v", v, ok)
	}
	again, err := Set(out, "Other", "profiles", "defaults", "font", "face")
	if err != nil || string(again) != strings.Replace(string(out), "JetBrainsMono NF", "Other", 1) {
		t.Fatalf("replace: %v\n%s", err, again)
	}
	// font exists without a face: the face goes first in it.
	src := strings.Replace(wt, `"defaults": {},`, "\"defaults\": {\n            \"font\": {\n                \"size\": 11\n            }\n        },", 1)
	out, err = Set([]byte(src), "F", "profiles", "defaults", "font", "face")
	if err != nil || !strings.Contains(string(out), "\"font\": {\n                \"face\": \"F\",\n                \"size\": 11") {
		t.Fatalf("into font: %v\n%s", err, out)
	}
	valid(t, out)
}

// TestSetVSCode: a top-level key goes first; comments, the BOM, a string
// holding "//" and trailing commas stay; CRLF files get CRLF.
func TestSetVSCode(t *testing.T) {
	out, err := Set([]byte(vscode), "JetBrainsMono NF", "terminal.integrated.fontFamily")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(vscode, "{\n", "{\n    \"terminal.integrated.fontFamily\": \"JetBrainsMono NF\",\n", 1)
	if string(out) != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out, want)
	}
	valid(t, out)
	if v, _ := Get(out, "a.b", "x"); v != "" {
		t.Fatalf("an array is not a string: %q", v)
	}
	crlf := strings.ReplaceAll(vscode, "\n", "\r\n")
	out, err = Set([]byte(crlf), "F", "terminal.integrated.fontFamily")
	if err != nil || !strings.Contains(string(out), "{\r\n    \"terminal.integrated.fontFamily\": \"F\",\r\n    // my editor") {
		t.Fatalf("crlf: %v\n%q", err, out)
	}
	out, err = Set([]byte("{}"), "F", "k")
	if err != nil || string(out) != "{\n    \"k\": \"F\"\n}" {
		t.Fatalf("empty file object: %v %q", err, out)
	}
}

// TestSetErrors: invalid JSON and a path through a non-object are refused.
func TestSetErrors(t *testing.T) {
	for _, src := range []string{"", "[]", `{"a": }`, `{"a": 1`, `{"profiles": []}`} {
		if _, err := Set([]byte(src), "F", "profiles", "defaults"); err == nil {
			t.Errorf("Set(%q) took it", src)
		}
	}
	if _, ok := Get([]byte(`{"a": {"b": 1}}`), "a", "b"); ok {
		t.Error("a number is not a string")
	}
	if v, ok := Get([]byte(`{"a": "x", "a": "y"}`), "a"); !ok || v != "y" {
		t.Errorf("the last duplicate wins: %q", v)
	}
}
