package cli

import (
	"strconv"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
)

// D90: the first interactive start sets up the Nerd Font once, unless over
// SSH, with emoji icons or IMUX_NO_FONT=1; a failure is tried again a day
// later.
func TestAutoFontWanted(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	emoji := &config.Config{}
	emoji.Tmux.Icons = "emoji"
	auto := &config.Config{}
	auto.Tmux.Icons = "auto"
	failed := func(ago time.Duration) []byte {
		return []byte("failed " + strconv.FormatInt(now.Add(-ago).Unix(), 10) + "\n")
	}
	done := func(ago time.Duration, ver string) []byte {
		return []byte("done " + strconv.FormatInt(now.Add(-ago).Unix(), 10) + " " + ver + "\n")
	}
	cases := []struct {
		name  string
		cfg   *config.Config
		env   func(string) string
		stamp []byte
		want  bool
	}{
		{"first start", auto, env(), nil, true},
		{"no config", nil, env(), nil, true},
		// Done for this version: not again; a new version looks again (D94).
		{"done for this version", auto, env(), done(time.Hour, "0.71.0"), false},
		{"done for an older version", auto, env(), done(time.Hour, "0.70.0"), true},
		{"done before versions were recorded", auto, env(), []byte("done\n"), true},
		{"IMUX_NO_FONT", auto, env("IMUX_NO_FONT", "1"), nil, false},
		{"over SSH", auto, env("SSH_CONNECTION", "10.0.0.1 22 10.0.0.2 22"), nil, false},
		{"SSH tty", auto, env("SSH_TTY", "/dev/pts/1"), nil, false},
		{"emoji icons", emoji, env(), nil, false},
		{"failed an hour ago", auto, env(), failed(time.Hour), false},
		{"failed a day ago", auto, env(), failed(25 * time.Hour), true},
		{"unreadable stamp", auto, env(), []byte("failed soon\n"), true},
	}
	for _, c := range cases {
		if got := autoFontWanted(c.cfg, c.env, c.stamp, now, "0.71.0"); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
