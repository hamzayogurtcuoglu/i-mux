package handoff

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// FileName returns the handoff file name for a switch at t.
func FileName(from, to session.AgentID, t time.Time) string {
	return t.Format("20060102-150405") + "-" + string(from) + "-to-" + string(to) + ".md"
}

// Save writes the document to ~/.imux/handoffs (dir 0700, file 0600) and
// returns its path. An existing file with the same name gets a suffix.
func Save(text string, from, to session.AgentID, t time.Time) (string, error) {
	dir := paths.Handoffs()
	if err := paths.EnsureDir(dir); err != nil {
		return "", err
	}
	name := FileName(from, to, t)
	p := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			break
		}
		p = filepath.Join(dir, strings.TrimSuffix(name, ".md")+"-"+itoa(i)+".md")
	}
	if err := paths.WriteFile(p, []byte(text)); err != nil {
		return "", err
	}
	return p, nil
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

// Prune deletes handoffs older than keepDays. It runs at most once a day,
// tracked by the mtime of ~/.imux/.pruned. It reports whether it ran.
func Prune(keepDays int, now time.Time) bool {
	if keepDays <= 0 {
		return false
	}
	stamp := paths.PruneStamp()
	if fi, err := os.Stat(stamp); err == nil && now.Sub(fi.ModTime()) < 24*time.Hour {
		return false
	}
	cutoff := now.Add(-time.Duration(keepDays) * 24 * time.Hour)
	ents, err := os.ReadDir(paths.Handoffs())
	if err == nil {
		for _, e := range ents {
			if !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			if fi, err := e.Info(); err == nil && fi.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(paths.Handoffs(), e.Name()))
			}
		}
	}
	if err := paths.EnsureDir(paths.Home()); err == nil {
		_ = paths.WriteFile(stamp, nil)
		_ = os.Chtimes(stamp, now, now)
	}
	return true
}
