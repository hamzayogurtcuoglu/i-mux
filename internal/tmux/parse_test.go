package tmux

import (
	"reflect"
	"testing"
)

func TestParseCommandLine(t *testing.T) {
	for in, want := range map[string][]Cmd{
		"switch-client -c '/dev/pts/1~p' -t %1~p ; select-pane -t %1~p": {
			{"switch-client", "-c", "/dev/pts/1~p", "-t", "%1~p"}, {"select-pane", "-t", "%1~p"},
		},
		`set-option -g @x "a \"b\" c\\d"`: {{"set-option", "-g", "@x", `a "b" c\d`}},
		`display 'it'\''s'`:               {{"display", "it's"}},
		"kill-pane":                       {{"kill-pane"}},
		"":                                nil,
	} {
		got, err := ParseCommandLine(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("ParseCommandLine(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseCommandLine("x 'open"); err == nil {
		t.Error("unterminated quote parsed")
	}
}
