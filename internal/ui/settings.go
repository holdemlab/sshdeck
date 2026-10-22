package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/holdemlab/sshdeck/internal/config"
)

type fieldKind int

const (
	fkText fieldKind = iota
	fkInt
	fkDuration
	fkBool
	fkKey    // captures a key press (prefix)
	fkList   // comma separated
	fkChoice // one of choices, switched with space / ← →
)

type setField struct {
	label string
	help  string
	kind  fieldKind
	input textinput.Model // text, int, duration, list
	on    bool            // bool
	key   string          // key
	// choice
	choices []string
	labels  map[string]string // shown names, translated
	choice  int
}

// settingsForm edits config.yaml.
type settingsForm struct {
	fields    []setField
	focus     int
	capturing bool // waiting for the new prefix key
	err       string
	orig      string // values when the screen opened
	discard   bool   // "discard changes?" is showing
}

// values is a comparable snapshot of every field.
func (f settingsForm) values() string {
	var b strings.Builder
	for _, fl := range f.fields {
		fmt.Fprintf(&b, "%s|%t|%s|%d\x00", fl.input.Value(), fl.on, fl.key, fl.choice)
	}
	return b.String()
}

func (f settingsForm) dirty() bool { return f.values() != f.orig }

const (
	sLanguage = iota
	sTheme
	sPrefix
	sSidebar
	sScrollback
	sTerm
	sTimeout
	sKeepalive
	sKnownHosts
	sAgent
	sIdentities
	sMouse
	sLocal
	sLocalCmd
	sRemember
	sConfirm
)

func newSettingsForm(s config.Settings) settingsForm {
	text := func(v, placeholder string) textinput.Model {
		in := textinput.New()
		in.Prompt = ""
		in.Placeholder = placeholder
		in.SetWidth(56)
		in.SetValue(v)
		in.CursorStart() // show the beginning of long values
		return in
	}
	dur := func(d config.Duration) string {
		if d == 0 {
			return "0"
		}
		return time.Duration(d).String()
	}
	f := settingsForm{fields: []setField{
		sLanguage:   {label: "Language", kind: fkChoice, choices: languageChoices, labels: languageLabels, choice: choiceIndex(languageChoices, s.Language), help: "auto follows $LANG · space or ← → to change"},
		sTheme:      {label: "Theme", kind: fkChoice, choices: themeNames, labels: themeLabels, choice: choiceIndex(themeNames, themeOf(s.Theme)), help: "Catppuccin flavour or the terminal's own colors · preview while choosing"},
		sPrefix:     {label: "Prefix key", kind: fkKey, key: s.Prefix, help: "enter, then press the new key (needs ctrl or alt)"},
		sSidebar:    {label: "Sidebar", kind: fkInt, input: text(strconv.Itoa(s.SidebarWidth), "32"), help: "width in columns, 16–120"},
		sScrollback: {label: "Scrollback", kind: fkInt, input: text(strconv.Itoa(s.Scrollback), "5000"), help: "lines kept per tab"},
		sTerm:       {label: "TERM", kind: fkText, input: text(s.Term, "xterm-256color"), help: "terminal type sent to servers (new tabs)"},
		sTimeout:    {label: "Timeout", kind: fkDuration, input: text(dur(s.ConnectTimeout), "15s"), help: "connect timeout per hop, e.g. 15s"},
		sKeepalive:  {label: "Keepalive", kind: fkDuration, input: text(dur(s.Keepalive), "30s"), help: "interval, 0 disables (new connections)"},
		sKnownHosts: {label: "known_hosts", kind: fkText, input: text(s.KnownHosts, "~/.config/sshdeck/known_hosts"), help: "sshdeck's own host key file"},
		sAgent:      {label: "ssh-agent", kind: fkBool, on: s.UseAgent, help: "use keys from $SSH_AUTH_SOCK"},
		sIdentities: {label: "Keys", kind: fkList, input: text(strings.Join(s.Identities, ", "), "~/.ssh/id_ed25519, ~/.ssh/id_rsa"), help: "key files tried for every session, comma separated"},
		sMouse:      {label: "Mouse", kind: fkBool, on: s.Mouse, help: "clicks, wheel, sidebar drag; select text with Shift held"},
		sLocal:      {label: "Local shell", kind: fkBool, on: s.LocalShell, help: "the \"local\" entry at the top of the tree (also prefix + l)"},
		sLocalCmd:   {label: "Local cmd", kind: fkText, input: text(s.LocalCommand, "$SHELL"), help: "runs in local tabs instead of $SHELL, e.g. zsh -l (new tabs)"},
		sRemember:   {label: "Remember pw", kind: fkBool, on: s.RememberPasswords, help: "keep accepted passwords / passphrases in memory until exit (never on disk)"},
		sConfirm:    {label: "Confirm close", kind: fkBool, on: s.ConfirmClose, help: "ask before closing a live tab"},
	}}
	f.setFocus(0)
	f.orig = f.values()
	return f
}

func (f *settingsForm) setFocus(i int) tea.Cmd {
	if fl := &f.fields[f.focus]; fl.kind != fkBool && fl.kind != fkKey && fl.kind != fkChoice {
		fl.input.Blur()
		fl.input.CursorStart() // blurred: show the beginning
	}
	f.focus = (i + len(f.fields)) % len(f.fields)
	f.capturing = false
	if fl := &f.fields[f.focus]; fl.kind != fkBool && fl.kind != fkKey && fl.kind != fkChoice {
		fl.input.CursorEnd() // editing: continue at the end
		return fl.input.Focus()
	}
	return nil
}

// result validates the inputs into Settings.
func (f settingsForm) result(base config.Settings) (config.Settings, error) {
	s := base
	val := func(i int) string { return strings.TrimSpace(f.fields[i].input.Value()) }
	num := func(i, min, max int) (int, error) {
		n, err := strconv.Atoi(val(i))
		if err != nil || n < min || n > max {
			return 0, fmt.Errorf("%s", T("%s must be a number from %d to %d", T(f.fields[i].label), min, max))
		}
		return n, nil
	}
	dur := func(i int) (config.Duration, error) {
		v := val(i)
		if v == "0" || v == "" {
			return 0, nil
		}
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return 0, fmt.Errorf("%s", T("%s: use a duration like 15s or 2m", T(f.fields[i].label)))
		}
		return config.Duration(d), nil
	}
	var err error
	s.Prefix = f.fields[sPrefix].key
	if err = config.ValidatePrefix(s.Prefix); err != nil {
		return s, err
	}
	if s.SidebarWidth, err = num(sSidebar, 16, 120); err != nil {
		return s, err
	}
	if s.Scrollback, err = num(sScrollback, 0, 1_000_000); err != nil {
		return s, err
	}
	if s.Term = val(sTerm); s.Term == "" {
		return s, fmt.Errorf("%s", T("TERM must not be empty"))
	}
	if s.ConnectTimeout, err = dur(sTimeout); err != nil {
		return s, err
	}
	if s.ConnectTimeout == 0 {
		return s, fmt.Errorf("%s", T("timeout must be more than 0"))
	}
	if s.Keepalive, err = dur(sKeepalive); err != nil {
		return s, err
	}
	if s.KnownHosts = val(sKnownHosts); s.KnownHosts == "" {
		return s, fmt.Errorf("%s", T("known_hosts path must not be empty"))
	}
	s.UseAgent = f.fields[sAgent].on
	s.Identities = nil
	for _, id := range strings.Split(val(sIdentities), ",") {
		if id = strings.TrimSpace(id); id != "" {
			s.Identities = append(s.Identities, id)
		}
	}
	s.ConfirmClose = f.fields[sConfirm].on
	s.Language = languageChoices[f.fields[sLanguage].choice]
	s.Theme = themeNames[f.fields[sTheme].choice]
	s.RememberPasswords = f.fields[sRemember].on
	s.LocalShell = f.fields[sLocal].on
	s.Mouse = f.fields[sMouse].on
	s.LocalCommand = strings.TrimSpace(val(sLocalCmd))
	return s, nil
}

var languageChoices = []string{"auto", "uk", "en"}

var languageLabels = map[string]string{"auto": "auto ($LANG)", "uk": "Українська", "en": "English"}

func choiceIndex(choices []string, v string) int {
	for i, c := range choices {
		if c == v {
			return i
		}
	}
	return 0
}

// themeOf normalizes a theme setting for the form.
func themeOf(s string) string {
	n, _ := normalizeTheme(s)
	return n
}

// previewTheme is the theme selected on the screen right now.
func (f settingsForm) previewTheme() string {
	if len(f.fields) <= sTheme {
		return ""
	}
	fl := f.fields[sTheme]
	return fl.choices[fl.choice]
}

type settingsAction int

const (
	saNone settingsAction = iota
	saSave
	saCancel
	saEditor
)

func (f settingsForm) update(msg tea.Msg) (settingsForm, tea.Cmd, settingsAction) {
	k, isKey := msg.(tea.KeyPressMsg)
	fl := &f.fields[f.focus]

	if isKey && f.capturing {
		s := k.String()
		f.capturing = false
		if s == "esc" {
			return f, nil, saNone
		}
		if err := config.ValidatePrefix(s); err != nil {
			f.err = err.Error()
			return f, nil, saNone
		}
		fl.key, f.err = s, ""
		return f, nil, saNone
	}
	if isKey && f.discard {
		f.discard = false
		switch k.String() {
		case "y":
			return f, nil, saCancel
		case "s", "ctrl+s":
			return f, nil, saSave
		}
		return f, nil, saNone
	}
	if isKey {
		switch k.String() {
		case "esc":
			if f.dirty() {
				f.discard = true
				return f, nil, saNone
			}
			return f, nil, saCancel
		case "ctrl+s":
			return f, nil, saSave
		case "ctrl+e":
			return f, nil, saEditor
		case "tab", "down":
			return f, f.setFocus(f.focus + 1), saNone
		case "shift+tab", "up":
			return f, f.setFocus(f.focus - 1), saNone
		case "enter":
			switch fl.kind {
			case fkKey:
				f.capturing, f.err = true, ""
				return f, nil, saNone
			case fkBool:
				fl.on = !fl.on
				return f, nil, saNone
			case fkChoice:
				fl.choice = (fl.choice + 1) % len(fl.choices)
				return f, nil, saNone
			}
			if f.focus == len(f.fields)-1 {
				return f, nil, saSave
			}
			return f, f.setFocus(f.focus + 1), saNone
		case "space", "left", "right", "h", "l", "y", "n":
			if fl.kind == fkChoice && len(fl.choices) > 0 {
				step := 1
				if k.String() == "left" || k.String() == "h" {
					step = -1
				}
				fl.choice = (fl.choice + step + len(fl.choices)) % len(fl.choices)
				return f, nil, saNone
			}
			if fl.kind == fkBool {
				switch k.String() {
				case "y":
					fl.on = true
				case "n":
					fl.on = false
				default:
					fl.on = !fl.on
				}
				return f, nil, saNone
			}
		}
	}
	if fl.kind == fkBool || fl.kind == fkKey || fl.kind == fkChoice {
		return f, nil, saNone
	}
	var cmd tea.Cmd
	fl.input, cmd = fl.input.Update(msg)
	return f, cmd, saNone
}

func (f settingsForm) view(path string) string {
	var b strings.Builder
	lw := 14 // label column, wide enough for the longest translated label
	for _, fl := range f.fields {
		lw = max(lw, lipgloss.Width(T(fl.label))+1)
	}
	b.WriteString(titleStyle.Render(T("Settings")) + mutedStyle.Render("  "+tildePath(path)) + "\n\n")
	accentStyle := lipgloss.NewStyle().Foreground(accent)
	for i, fl := range f.fields {
		marker := "  "
		if i == f.focus {
			marker = accentStyle.Render("▸ ")
		}
		var value string
		switch fl.kind {
		case fkBool:
			value = mutedStyle.Render("[ ] " + T("off"))
			if fl.on {
				value = okStyle.Render("[x] " + T("on"))
			}
		case fkChoice:
			var opts []string
			for j, c := range fl.choices {
				label := T(fl.labels[c])
				if j == fl.choice {
					opts = append(opts, okStyle.Render("● "+label))
				} else {
					opts = append(opts, mutedStyle.Render("○ "+label))
				}
			}
			value = strings.Join(opts, "  ")
		case fkKey:
			value = warnStyle.Render(fl.key)
			if i == f.focus && f.capturing {
				value = prefixStyle.Render(T("press the new key…")) + mutedStyle.Render("  "+T("esc keeps %s", fl.key))
			}
		default:
			value = fl.input.View()
		}
		b.WriteString(marker + labelStyle.Width(lw).Render(T(fl.label)) + " " + value + "\n")
		if i == f.focus {
			b.WriteString(strings.Repeat(" ", lw+3) + mutedStyle.Render(T(fl.help)) + "\n")
		}
	}
	b.WriteString("\n")
	if f.err != "" {
		b.WriteString(errStyle.Render("✗ "+f.err) + "\n")
	}
	if f.discard {
		b.WriteString(prefixStyle.Render(T(discardHint)) + "\n")
		return b.String()
	}
	b.WriteString(mutedStyle.Render(T("tab/↑↓ move · enter edit/toggle · ctrl+s save · esc cancel")) + "\n")
	b.WriteString(mutedStyle.Render(T("ctrl+e edit config.yaml in $EDITOR")) + "\n\n")
	b.WriteString(mutedStyle.Render(T("Language, prefix, theme, sidebar, scrollback apply at once;")) + "\n")
	b.WriteString(mutedStyle.Render(T("TERM, keys, timeouts on the next connection.")))
	return b.String()
}

// tildePath shortens $HOME to ~ for display.
func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "/" && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

// ---- external editor ----

type editTarget int

const (
	editSettings editTarget = iota
	editSessions
)

type editorDoneMsg struct {
	target editTarget
	err    error
}

// editorCommand builds the command for $VISUAL / $EDITOR (default vi).
func editorCommand(path string) *exec.Cmd {
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed == "" {
		ed = "vi"
	}
	parts := strings.Fields(ed) // allows "code --wait" etc.
	return exec.Command(parts[0], append(parts[1:], path)...)
}

func openEditor(path string, target editTarget) tea.Cmd {
	if target == editSessions {
		// let the editor open an empty file on first use
		if _, err := os.Stat(path); os.IsNotExist(err) {
			_ = config.Save(path, &config.Config{})
		}
	}
	return tea.ExecProcess(editorCommand(path), func(err error) tea.Msg {
		return editorDoneMsg{target: target, err: err}
	})
}
