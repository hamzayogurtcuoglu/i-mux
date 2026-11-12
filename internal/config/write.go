package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Str formats s as a TOML string value.
func Str(s string) string { return strconv.Quote(s) }

// Bool formats b as a TOML boolean value.
func Bool(b bool) string { return strconv.FormatBool(b) }

// ModTime is the config file's modification time (zero when missing); the
// sidebars and the watcher reload a config whose time changed.
func ModTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// Set writes `key = value` (value already in TOML form: Str, Bool, a
// number) into the file at path, in [section] ("" = the top level). Only
// that key's line changes; comments and other lines stay. A missing file
// is written from Template first. The result must load, or the file is
// left as it was.
func Set(path, section, key, value string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		b = []byte(Template)
	} else if err != nil {
		return err
	}
	out := setLine(string(b), section, key, value)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(out); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, _, err := Load(tmp.Name()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

var headerRe = regexp.MustCompile(`^\s*\[([A-Za-z0-9_.\-]+)\]\s*(#.*)?$`)

// setLine sets key in section of src (see Set).
func setLine(src, section, key, value string) string {
	keyRe := regexp.MustCompile(`^(\s*` + regexp.QuoteMeta(key) + `\s*=\s*)`)
	lines := strings.SplitAfter(src, "\n")
	cur := ""
	last := -1 // the last non-blank line of the section (or its header)
	firstHeader := -1
	for i, l := range lines {
		if m := headerRe.FindStringSubmatch(l); m != nil {
			cur = m[1]
			if firstHeader < 0 {
				firstHeader = i
			}
			if cur == section {
				last = i
			}
			continue
		}
		if cur != section {
			continue
		}
		if m := keyRe.FindString(l); m != "" {
			nl := ""
			if strings.HasSuffix(l, "\n") {
				nl = "\n"
			}
			lines[i] = m + value + nl
			return strings.Join(lines, "")
		}
		if strings.TrimSpace(l) != "" {
			last = i
		}
	}
	line := key + " = " + value + "\n"
	switch {
	case section == "" && firstHeader >= 0:
		at := firstHeader
		if last >= 0 {
			at = last + 1
		}
		lines = append(lines[:at], append([]string{line}, lines[at:]...)...)
	case section == "" || last >= 0:
		at := len(lines)
		if last >= 0 {
			at = last + 1
		}
		lines = append(lines[:at], append([]string{line}, lines[at:]...)...)
	default:
		out := strings.Join(lines, "")
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		return out + "\n[" + section + "]\n" + line
	}
	return strings.Join(lines, "")
}
