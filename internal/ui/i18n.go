package ui

import "github.com/holdemlab/sshdeck/internal/i18n"

// T translates an interface message (see package i18n).
func T(s string, args ...any) string { return i18n.T(s, args...) }
