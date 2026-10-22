// Package docs embeds the in-app help.
package docs

import _ "embed"

// Help is help.md: the in-app help (prefix + ?) in English.
//
//go:embed help.md
var Help string

// HelpUK is uk/help.md: the same help in Ukrainian.
//
//go:embed uk/help.md
var HelpUK string

// HelpFor returns the help for a language ("uk" or "en").
func HelpFor(lang string) string {
	if lang == "uk" {
		return HelpUK
	}
	return Help
}
