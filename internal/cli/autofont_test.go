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
	cases := []struct {
		name  string
		cfg   *config.Config
		env   func(string) string
		stamp []byte
		want  bool
	}{
		{"first start", auto, env(), nil, true},
		{"no config", nil, env(), nil, true},
		{"done", auto, env(), []byte("done\n"), false},
		{"IMUX_NO_FONT", auto, env("IMUX_NO_FONT", "1"), nil, false},
		{"over SSH", auto, env("SSH_CONNECTION", "10.0.0.1 22 10.0.0.2 22"), nil, false},
		{"SSH tty", auto, env("SSH_TTY", "/dev/pts/1"), nil, false},
		{"emoji icons", emoji, env(), nil, false},
		{"failed an hour ago", auto, env(), failed(time.Hour), false},
		{"failed a day ago", auto, env(), failed(25 * time.Hour), true},
		{"unreadable stamp", auto, env(), []byte("failed soon\n"), true},
	}
	for _, c := range cases {
		if got := autoFontWanted(c.cfg, c.env, c.stamp, now); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
