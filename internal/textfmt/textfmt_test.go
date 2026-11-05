package textfmt

import (
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

func TestAgo(t *testing.T) {
	i18n.SetLocale("en")
	now := time.Date(2026, 11, 5, 14, 0, 0, 0, time.Local)
	cases := map[time.Duration]string{
		10 * time.Second:    "just now",
		12 * time.Minute:    "12 min ago",
		2 * time.Hour:       "2 h ago",
		26 * time.Hour:      "yesterday",
		3 * 24 * time.Hour:  "3 days ago",
		20 * 24 * time.Hour: "Oct 16",
	}
	for d, want := range cases {
		if got := Ago(now, now.Add(-d)); got != want {
			t.Errorf("%v: %q want %q", d, got, want)
		}
	}
}

func TestSize(t *testing.T) {
	for n, want := range map[int64]string{812: "812 B", 4300: "4.2 KB", 67_000_000: "63.9 MB"} {
		if got := Size(n); got != want {
			t.Errorf("%d: %q", n, got)
		}
	}
}
