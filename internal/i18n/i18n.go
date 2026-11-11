// Package i18n holds every user-facing UI string in one message catalog.
//
// English is the reference catalog. Other locales fall back to English key
// by key. The handoff preamble is model-facing and therefore always English;
// it does not go through this package.
package i18n

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
)

var catalogs = map[string]map[string]string{
	"en": en,
	"tr": tr,
}

var (
	mu      sync.RWMutex
	current = "en"
)

// Resolve picks the locale: the config value first, then IMUX_LANG, then LANG
// (e.g. "tr_TR.UTF-8" → "tr"). Unknown locales resolve to "en".
func Resolve(configLocale, imuxLang, lang string) string {
	for _, c := range []string{configLocale, imuxLang, lang} {
		c = normalize(c)
		if c == "" {
			continue
		}
		if _, ok := catalogs[c]; ok {
			return c
		}
		return "en"
	}
	return "en"
}

func normalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "C" || s == "POSIX" {
		return ""
	}
	if i := strings.IndexAny(s, "_.-@"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}

// Init sets the process locale from the config value and the environment.
func Init(configLocale string) {
	SetLocale(Resolve(configLocale, os.Getenv("IMUX_LANG"), os.Getenv("LANG")))
}

// SetLocale sets the current locale ("en", "tr", ...).
func SetLocale(l string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := catalogs[l]; ok {
		current = l
	} else {
		current = "en"
	}
}

// Locale returns the current locale.
func Locale() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// T returns the message for key in the current locale, formatted with args
// (fmt verbs). Missing keys return the key itself so that the UI never breaks.
//
// A key with a platform variant ("doctor.tmux_missing@windows") gives that
// variant on that platform: the same message with the platform's own
// commands, and the same fmt verbs.
func T(key string, args ...any) string {
	mu.RLock()
	loc := current
	mu.RUnlock()
	f, ok := lookup(loc, key+"@"+OS)
	if !ok {
		f, ok = lookup(loc, key)
	}
	if !ok {
		return key
	}
	if len(args) == 0 {
		return f
	}
	return fmt.Sprintf(f, args...)
}

// OS selects the platform variants of keys (runtime.GOOS); tests replace it.
var OS = runtime.GOOS

// lookup finds key in locale loc, else in English.
func lookup(loc, key string) (string, bool) {
	if f, ok := catalogs[loc][key]; ok {
		return f, true
	}
	f, ok := en[key]
	return f, ok
}

// Has reports whether key exists in the English catalog.
func Has(key string) bool {
	_, ok := en[key]
	return ok
}

// Keys returns the English catalog's keys, sorted.
func Keys() []string {
	ks := make([]string, 0, len(en))
	for k := range en {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Locales returns the available locales, sorted.
func Locales() []string {
	ls := make([]string, 0, len(catalogs))
	for l := range catalogs {
		ls = append(ls, l)
	}
	sort.Strings(ls)
	return ls
}

// Catalog returns the raw catalog of a locale (nil if unknown). For tests.
func Catalog(locale string) map[string]string { return catalogs[locale] }
