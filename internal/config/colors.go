package config

import (
	"fmt"
	"strings"
)

// ColorNames are the colors a session or folder can be marked with: the
// Catppuccin palette names. The interface draws them with the active theme
// (or a Catppuccin flavour matching the terminal background).
var ColorNames = []string{
	"rosewater", "flamingo", "pink", "mauve", "red", "maroon", "peach", "yellow",
	"green", "teal", "sky", "sapphire", "blue", "lavender",
	"text", "subtext", "overlay", "surface",
}

var colorAliases = map[string]string{
	"orange": "peach", "purple": "mauve", "violet": "mauve", "cyan": "teal",
	"gray": "overlay", "grey": "overlay", "white": "text", "magenta": "pink",
}

// NormalizeColor returns the canonical form of a color value: "" (inherit),
// "none", a palette name or "#rrggbb" (lower case). ok is false when the
// value is not a color.
func NormalizeColor(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "none" {
		return s, true
	}
	if a, ok := colorAliases[s]; ok {
		s = a
	}
	for _, n := range ColorNames {
		if s == n {
			return s, true
		}
	}
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if len(h) == 6 && strings.Trim(h, "0123456789abcdef") == "" {
			return "#" + h, true
		}
	}
	return "", false
}

func (d *Defaults) validateColors() error {
	for _, f := range []*string{&d.Color, &d.Fill} {
		n, ok := NormalizeColor(*f)
		if !ok {
			return fmt.Errorf("color %q: use a palette name (%s), #rrggbb or none", *f, strings.Join(ColorNames, ", "))
		}
		*f = n
	}
	return nil
}

// FolderDefaults returns the settings in effect for the folder at path,
// with everything inherited from above applied.
func (c *Config) FolderDefaults(path []string) Defaults {
	d := c.Defaults
	folders := c.Folders
	for _, name := range path {
		f := findFolder(folders, name)
		if f == nil {
			break
		}
		d = merge(d, f.Defaults)
		folders = f.Folders
	}
	return d
}
