// Package i18n translates sshdeck's interface. English text is the key;
// other languages map it to a translation (missing entries fall back to
// English). The language is process-wide and can change at run time.
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// Lang is an interface language.
type Lang string

const (
	EN Lang = "en"
	UK Lang = "uk"
)

var current atomic.Value // Lang

func init() { current.Store(EN) }

// Set switches the interface language.
func Set(l Lang) { current.Store(l) }

// Current returns the interface language.
func Current() Lang { return current.Load().(Lang) }

// Resolve turns a setting ("auto", "uk", "en") into a language. "auto"
// follows the locale: LC_ALL, then LC_MESSAGES, then LANG; uk_* is
// Ukrainian, anything else English.
func Resolve(setting string) Lang {
	switch strings.ToLower(strings.TrimSpace(setting)) {
	case "uk", "ua", "ukrainian", "українська":
		return UK
	case "en", "english":
		return EN
	}
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if val := os.Getenv(v); val != "" {
			if strings.HasPrefix(strings.ToLower(val), "uk") {
				return UK
			}
			return EN
		}
	}
	return EN
}

// Valid reports whether s is an accepted language setting.
func Valid(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto", "uk", "en":
		return true
	}
	return false
}

// T translates s (a format string when args are given).
func T(s string, args ...any) string {
	if Current() == UK {
		if tr, ok := uk[s]; ok {
			s = tr
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Has reports whether s has a translation for l (English always has).
func Has(l Lang, s string) bool {
	if l == EN {
		return true
	}
	_, ok := uk[s]
	return ok
}
