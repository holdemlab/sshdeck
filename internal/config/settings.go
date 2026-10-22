package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/holdemlab/sshdeck/internal/i18n"
)

// Settings are application-wide options from config.yaml.
type Settings struct {
	// Language of the interface and the built-in help: auto (from $LANG),
	// uk or en.
	Language string `yaml:"language"`
	// Theme: terminal (the terminal's own palette) or a Catppuccin flavour:
	// latte, frappe, macchiato, mocha.
	Theme string `yaml:"theme"`

	// Prefix is the key that starts a command while a terminal tab has
	// focus (tmux style). Pressing it twice sends it to the remote side.
	Prefix string `yaml:"prefix"`

	// SidebarWidth is the width of the session tree, in columns.
	SidebarWidth int `yaml:"sidebar_width"`

	// Scrollback is the number of lines each tab keeps above the screen.
	Scrollback int `yaml:"scrollback"`

	// Term is the TERM value requested for the remote pty.
	Term string `yaml:"term"`

	// ConnectTimeout limits TCP connect + SSH handshake per hop.
	ConnectTimeout Duration `yaml:"connect_timeout"`

	// Keepalive is the interval for keepalive requests; 0 disables them.
	Keepalive Duration `yaml:"keepalive"`

	// KnownHosts is sshdeck's own known_hosts file.
	KnownHosts string `yaml:"known_hosts"`

	// UseAgent enables keys from ssh-agent ($SSH_AUTH_SOCK).
	UseAgent bool `yaml:"use_agent"`

	// Identities are key files tried for every session, after the
	// session's own key. Missing files are skipped.
	Identities []string `yaml:"identities"`

	// ConfirmClose asks before closing a tab with a live connection.
	ConfirmClose bool `yaml:"confirm_close"`

	// RememberPasswords keeps accepted passwords and key passphrases in
	// memory until sshdeck exits (never on disk).
	RememberPasswords bool `yaml:"remember_passwords"`

	// Mouse lets sshdeck use the mouse: clicks on tabs and the tree, the
	// wheel, dragging the sidebar edge, and mouse input for programs in the
	// tabs. Text is then selected with Shift held.
	Mouse bool `yaml:"mouse"`

	// LocalShell shows the built-in "local" entry at the top of the tree: a
	// tab with a shell on this computer, no SSH involved.
	LocalShell bool `yaml:"local_shell"`
	// LocalCommand runs in local tabs instead of $SHELL ("zsh -l", "fish").
	LocalCommand string `yaml:"local_command"`
	// LocalColor and LocalFill mark the local entry like a session's
	// color / fill.
	LocalColor string `yaml:"local_color"`
	LocalFill  string `yaml:"local_fill"`
}

// DefaultSettings returns the built-in defaults.
func DefaultSettings() Settings {
	return Settings{
		Language:       "auto",
		Theme:          "terminal",
		Prefix:         "ctrl+a",
		SidebarWidth:   32,
		Scrollback:     5000,
		Term:           "xterm-256color",
		ConnectTimeout: Duration(15 * time.Second),
		Keepalive:      Duration(30 * time.Second),
		KnownHosts:     "~/.config/sshdeck/known_hosts",
		UseAgent:       true,
		Identities:     []string{"~/.ssh/id_ed25519", "~/.ssh/id_ecdsa", "~/.ssh/id_rsa"},
		ConfirmClose:   true,

		RememberPasswords: true,
		LocalShell:        true,
		Mouse:             true,
	}
}

// Render formats settings as a commented config.yaml. sshdeck writes the
// file with this on first start and whenever settings are saved from the UI,
// so the explanations survive edits.
func (s Settings) Render() string {
	var ids strings.Builder
	if len(s.Identities) == 0 {
		ids.WriteString(" []\n")
	} else {
		ids.WriteString("\n")
		for _, id := range s.Identities {
			ids.WriteString("  - " + strconv.Quote(id) + "\n")
		}
	}
	tmpl := settingsTemplateEN
	if i18n.Resolve(s.Language) == i18n.UK {
		tmpl = settingsTemplateUK
	}
	lang := s.Language
	if lang == "" {
		lang = "auto"
	}
	th := s.Theme
	if th == "" {
		th = "terminal"
	}
	return fmt.Sprintf(tmpl, lang, th, strconv.Quote(s.Prefix), s.SidebarWidth, s.Scrollback, strconv.Quote(s.Term),
		durString(s.ConnectTimeout), durString(s.Keepalive), strconv.Quote(s.KnownHosts),
		s.UseAgent, ids.String(), s.RememberPasswords, s.ConfirmClose, s.Mouse,
		s.LocalShell, strconv.Quote(s.LocalCommand), strconv.Quote(s.LocalColor), strconv.Quote(s.LocalFill))
}

// The comments of config.yaml follow the interface language.
const settingsTemplateEN = `# sshdeck settings. Sessions live in sessions.yaml next to this file.
# Edit here, or inside sshdeck with prefix + , (comma).

# Interface and help language: auto (from $LANG), uk or en.
language: %s

# Colors: terminal (your terminal's own palette) or a Catppuccin flavour:
# latte (light), frappe, macchiato, mocha (darkest).
theme: %s

# Key that starts a command in a terminal tab (press it twice to send it
# to the remote side). Then: 1-9 tab, n/p next/prev, s sidebar, t tunnels,
# w close, r reconnect, [ scroll, , settings, ? help, q quit.
# Needs ctrl or alt, e.g. ctrl+a, ctrl+], alt+a.
prefix: %s

sidebar_width: %d
scrollback: %d            # lines kept per tab
term: %s

connect_timeout: %s
keepalive: %s             # 0 disables keepalives

# sshdeck keeps its own known_hosts and never reads ~/.ssh/config.
known_hosts: %s

# Authentication: the session's key, then ssh-agent, then these keys,
# then password / keyboard-interactive (asked in the tab).
use_agent: %t
identities:%s
# Remember accepted passwords and key passphrases in memory until sshdeck
# exits, so reconnects and new tabs do not ask again. Never written to disk.
remember_passwords: %t

confirm_close: %t         # ask before closing a tab with a live connection

# Mouse: click tabs and the tree, scroll with the wheel, drag the sidebar
# edge; programs in the tabs (vim, htop, mc) get the mouse too. While it is
# on, select text with Shift held (Option in iTerm2).
mouse: %t

# The "local" entry at the top of the tree opens a shell on this computer
# (also prefix + l). local_command runs instead of $SHELL, e.g. "zsh -l".
# local_color / local_fill mark it like a session (palette name or #rrggbb).
local_shell: %t
local_command: %s
local_color: %s
local_fill: %s
`

const settingsTemplateUK = `# Налаштування sshdeck. Сесії лежать у sessions.yaml поруч із цим файлом.
# Редагуй тут або в самому sshdeck: prefix + , (кома).

# Мова інтерфейсу й довідки: auto (за $LANG), uk або en.
language: %s

# Кольори: terminal (палітра твого термінала) або варіант Catppuccin:
# latte (світлий), frappe, macchiato, mocha (найтемніший).
theme: %s

# Клавіша, з якої у вкладці починається команда (двічі — надіслати її
# на сервер). Далі: 1-9 вкладка, n/p наступна/попередня, s сайдбар,
# t тунелі, w закрити, r перепідключити, [ історія, , налаштування,
# ? довідка, q вийти. Потрібен ctrl або alt, напр. ctrl+a, ctrl+], alt+a.
prefix: %s

sidebar_width: %d
scrollback: %d            # скільки рядків історії на вкладку
term: %s

connect_timeout: %s
keepalive: %s             # 0 вимикає keepalive

# sshdeck має власний known_hosts і ніколи не читає ~/.ssh/config.
known_hosts: %s

# Авторизація: ключ сесії, потім ssh-agent, потім ці ключі,
# потім пароль / keyboard-interactive (питає у вкладці).
use_agent: %t
identities:%s
# Пам'ятати прийняті паролі й passphrase ключів у пам'яті до виходу з
# sshdeck, щоб перепідключення й нові вкладки не питали знову. На диск не пишуться.
remember_passwords: %t

confirm_close: %t         # питати перед закриттям вкладки з живим з'єднанням

# Миша: клік по вкладках і дереву, прокрутка колесом, перетягування межі
# сайдбара; програми у вкладках (vim, htop, mc) теж отримують мишу. Поки вона
# увімкнена, текст виділяють із затиснутим Shift (Option в iTerm2).
mouse: %t

# Пункт «local» угорі дерева відкриває shell на цьому комп'ютері (також
# prefix + l). local_command запускається замість $SHELL, напр. "zsh -l".
# local_color / local_fill позначають його, як сесію (назва з палітри або #rrggbb).
local_shell: %t
local_command: %s
local_color: %s
local_fill: %s
`

func durString(d Duration) string {
	if d == 0 {
		return "0"
	}
	return time.Duration(d).String() // "30s", "1m30s": ParseDuration reads these back
}

// SaveSettings validates s and writes it to path (commented, atomically).
func SaveSettings(path string, s Settings) error {
	if err := s.validate(); err != nil {
		return err
	}
	return writeAtomic(path, []byte(s.Render()))
}

// SettingsPath returns the default config.yaml location.
func SettingsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// LoadSettings reads config.yaml over the defaults. If the file does not
// exist and create is true, a commented template is written.
func LoadSettings(path string, create bool) (Settings, error) {
	s := DefaultSettings()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if create {
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
				_ = os.WriteFile(path, []byte(s.Render()), 0o600)
			}
		}
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := s.validate(); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

func (s *Settings) validate() error {
	if err := ValidatePrefix(s.Prefix); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(s.Language)) {
	case "", "auto":
		s.Language = "auto"
	case "uk", "en":
		s.Language = strings.ToLower(strings.TrimSpace(s.Language))
	default:
		return fmt.Errorf("language %q: use auto, uk or en", s.Language)
	}
	switch t := strings.ToLower(strings.TrimSpace(s.Theme)); t {
	case "", "terminal":
		s.Theme = "terminal"
	case "latte", "frappe", "macchiato", "mocha":
		s.Theme = t
	case "frappé":
		s.Theme = "frappe"
	default:
		return fmt.Errorf("theme %q: use terminal, latte, frappe, macchiato or mocha", s.Theme)
	}
	if s.SidebarWidth < 16 {
		s.SidebarWidth = 16
	}
	if s.SidebarWidth > 120 {
		s.SidebarWidth = 120
	}
	if s.Scrollback < 0 {
		s.Scrollback = 0
	}
	if s.Term == "" {
		s.Term = "xterm-256color"
	}
	if s.ConnectTimeout <= 0 {
		s.ConnectTimeout = Duration(15 * time.Second)
	}
	for _, c := range []*string{&s.LocalColor, &s.LocalFill} {
		n, ok := NormalizeColor(*c)
		if !ok {
			return fmt.Errorf("local_color / local_fill %q: use a palette name, #rrggbb or nothing", *c)
		}
		*c = n
	}
	s.LocalCommand = strings.TrimSpace(s.LocalCommand)
	return nil
}

// ValidatePrefix checks that the prefix key has a ctrl or alt modifier, so
// it cannot swallow ordinary typing.
func ValidatePrefix(p string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return errors.New("prefix must not be empty")
	}
	if !strings.Contains(p, "ctrl+") && !strings.Contains(p, "alt+") {
		return fmt.Errorf("prefix %q needs ctrl or alt (e.g. ctrl+a, ctrl+], alt+a)", p)
	}
	if p == "ctrl+c" || p == "ctrl+m" || p == "ctrl+i" || p == "ctrl+[" {
		return fmt.Errorf("prefix %q would break normal input", p)
	}
	return nil
}

// Duration is a time.Duration that reads "30s" / "2m" from YAML.
type Duration time.Duration

func (d Duration) Std() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if n.Value == "0" {
		*d = 0
		return nil
	}
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: invalid duration %q (use e.g. 30s, 2m)", n.Line, n.Value)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

// ExpandHome replaces a leading "~/" with the user's home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
