// Package i18n holds every user-facing UI string in one message catalog.
//
// English is the reference catalog. Other locales fall back to English key
// by key. The handoff preamble is model-facing and therefore always English;
// it does not go through this package.
package i18n

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

var catalogs = map[string]map[string]string{
	"en": en,
}

var (
	mu      sync.RWMutex
	current = "en"
)

// Resolve picks the locale: the config value first, then AJAN_LANG, then LANG
// (e.g. "tr_TR.UTF-8" → "tr"). Unknown locales resolve to "en".
func Resolve(configLocale, ajanLang, lang string) string {
	for _, c := range []string{configLocale, ajanLang, lang} {
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
	SetLocale(Resolve(configLocale, os.Getenv("AJAN_LANG"), os.Getenv("LANG")))
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
func T(key string, args ...any) string {
	mu.RLock()
	loc := current
	mu.RUnlock()
	f, ok := catalogs[loc][key]
	if !ok {
		f, ok = en[key]
	}
	if !ok {
		return key
	}
	if len(args) == 0 {
		return f
	}
	return fmt.Sprintf(f, args...)
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
