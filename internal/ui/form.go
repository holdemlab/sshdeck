package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/tunnel"
)

const (
	fName = iota
	fFolder
	fHost
	fUser
	fPort
	fJump
	fIdentity
	fCommand
	fForwards
	fNote
	fColor
	fFill
	fieldCount
)

var fieldLabels = [fieldCount]string{"Name", "Folder", "Host", "User", "Port", "Jump", "Key file", "Command", "Tunnels", "Note", "Color", "Fill"}
var fieldHints = [fieldCount]string{
	"e.g. web-01",
	"e.g. prod/web  (created if missing)",
	"hostname or IP",
	"empty = inherit",
	"empty = inherit / 22",
	"session path (infra/proxy) or user@host:port",
	"e.g. ~/.ssh/id_ed25519  (empty = inherit)",
	"run instead of the shell, e.g. htop",
	"L 8080:localhost:80, D 1080  (comma separated)",
	"",
	"← → choose · or type #rrggbb",
	"← → choose · or type #rrggbb",
}

// form edits one session, or one folder (folderMode). editing is the
// original location when an existing item is edited; nil for a new one.
type form struct {
	inputs     [fieldCount]textinput.Model
	active     []int // fields shown, in order
	focus      int   // index into active
	editing    *sessionRef
	folderMode bool
	err        string
	title      string

	orig    [fieldCount]string // values when the form opened
	discard bool               // "discard changes?" is showing
}

// discardHint is shown when esc is pressed on something with unsaved input.
const discardHint = "Discard changes?  y discard · s save · n / esc keep editing" // translated where shown

func (f *form) snapshot() {
	for i := range f.inputs {
		f.orig[i] = f.inputs[i].Value()
	}
}

// dirty reports whether anything was typed since the form opened.
func (f form) dirty() bool {
	for i := range f.inputs {
		if f.inputs[i].Value() != f.orig[i] {
			return true
		}
	}
	return false
}

var (
	sessionFields = []int{fName, fFolder, fHost, fUser, fPort, fJump, fIdentity, fCommand, fForwards, fColor, fFill, fNote}
	folderFields  = []int{fName, fFolder, fUser, fPort, fJump, fIdentity, fColor, fFill}
)

func (f form) label(i int) string {
	if f.folderMode && i == fFolder {
		return T("Inside")
	}
	return T(fieldLabels[i])
}

type sessionRef struct {
	path []string
	name string
}

func newForm(title string, path []string, s *config.Session, editing *sessionRef) form {
	f := form{title: title, editing: editing, active: sessionFields}
	for i := range f.inputs {
		in := textinput.New()
		in.Prompt = ""
		in.Placeholder = T(fieldHints[i])
		in.SetWidth(48)
		f.inputs[i] = in
	}
	f.inputs[fFolder].SetValue(strings.Join(path, "/"))
	if s != nil {
		f.inputs[fName].SetValue(s.Name)
		f.inputs[fHost].SetValue(s.Host)
		f.inputs[fUser].SetValue(s.User)
		if s.Port != 0 {
			f.inputs[fPort].SetValue(strconv.Itoa(s.Port))
		}
		f.inputs[fJump].SetValue(s.Jump)
		f.inputs[fIdentity].SetValue(s.Identity)
		f.inputs[fCommand].SetValue(s.Command)
		f.inputs[fForwards].SetValue(strings.Join(s.Forwards, ", "))
		f.inputs[fNote].SetValue(s.Note)
		f.inputs[fColor].SetValue(s.Color)
		f.inputs[fFill].SetValue(s.Fill)
	}
	f.inputs[f.active[0]].Focus()
	f.snapshot()
	return f
}

// newFolderForm edits the folder at path (fd != nil) or creates one inside
// path (fd == nil).
func newFolderForm(title string, path []string, fd *config.Folder) form {
	f := form{title: title, folderMode: true, active: folderFields}
	for i := range f.inputs {
		in := textinput.New()
		in.Prompt = ""
		in.Placeholder = T(fieldHints[i])
		in.SetWidth(48)
		f.inputs[i] = in
	}
	f.inputs[fName].Placeholder = T("folder name, e.g. prod")
	f.inputs[fFolder].Placeholder = T("parent folder, empty = top level")
	f.inputs[fUser].Placeholder = T("inherited by everything inside")
	f.inputs[fJump].Placeholder = T("e.g. infra/proxy — used by everything inside")
	parent := path
	if fd != nil {
		parent = path[:len(path)-1]
		f.editing = &sessionRef{path: path, name: fd.Name}
		f.inputs[fName].SetValue(fd.Name)
		f.inputs[fUser].SetValue(fd.User)
		if fd.Port != 0 {
			f.inputs[fPort].SetValue(strconv.Itoa(fd.Port))
		}
		f.inputs[fJump].SetValue(fd.Jump)
		f.inputs[fIdentity].SetValue(fd.Identity)
		f.inputs[fColor].SetValue(fd.Color)
		f.inputs[fFill].SetValue(fd.Fill)
	}
	f.inputs[fFolder].SetValue(strings.Join(parent, "/"))
	f.inputs[fName].Focus()
	f.snapshot()
	return f
}

// folderResult reads a folder form: the new full path and its defaults.
func (f form) folderResult() ([]string, config.Defaults, error) {
	v := func(i int) string { return strings.TrimSpace(f.inputs[i].Value()) }
	var d config.Defaults
	name := v(fName)
	if name == "" {
		return nil, d, fmt.Errorf("%s", T("name is required"))
	}
	if strings.Contains(name, "/") {
		return nil, d, fmt.Errorf("%s", T("name must not contain / (use Inside to nest)"))
	}
	d.User, d.Jump, d.Identity = v(fUser), v(fJump), v(fIdentity)
	var err error
	if d.Color, d.Fill, err = f.colors(); err != nil {
		return nil, d, err
	}
	if p := v(fPort); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, d, fmt.Errorf("%s", T("port must be 1-65535"))
		}
		d.Port = n
	}
	return append(config.SplitPath(v(fFolder)), name), d, nil
}

// result builds the session and its folder path from the inputs.
func (f form) result() ([]string, *config.Session, error) {
	v := func(i int) string { return strings.TrimSpace(f.inputs[i].Value()) }
	s := &config.Session{
		Name:    v(fName),
		Host:    v(fHost),
		Command: v(fCommand),
		Note:    v(fNote),
	}
	s.User = v(fUser)
	s.Jump = v(fJump)
	s.Identity = v(fIdentity)
	var err error
	if s.Color, s.Fill, err = f.colors(); err != nil {
		return nil, nil, err
	}
	if s.Name == "" {
		return nil, nil, fmt.Errorf("%s", T("name is required"))
	}
	if strings.Contains(s.Name, "/") {
		return nil, nil, fmt.Errorf("%s", T("name must not contain /"))
	}
	if s.Host == "" {
		return nil, nil, fmt.Errorf("%s", T("host is required"))
	}
	for _, fw := range strings.Split(v(fForwards), ",") {
		if fw = strings.TrimSpace(fw); fw == "" {
			continue
		}
		sp, err := tunnel.Parse(fw)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %v", T("tunnels"), err)
		}
		s.Forwards = append(s.Forwards, sp.String())
	}
	if p := v(fPort); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, nil, fmt.Errorf("%s", T("port must be 1-65535"))
		}
		s.Port = n
	}
	return config.SplitPath(v(fFolder)), s, nil
}

// colors reads and checks the color fields.
func (f form) colors() (colorV, fillV string, err error) {
	out := [2]string{}
	for j, i := range []int{fColor, fFill} {
		n, ok := config.NormalizeColor(f.inputs[i].Value())
		if !ok {
			return "", "", fmt.Errorf("%s: %s", T(fieldLabels[i]), T("not a color: %s", strings.TrimSpace(f.inputs[i].Value())))
		}
		out[j] = n
	}
	return out[0], out[1], nil
}

func isColorField(i int) bool { return i == fColor || i == fFill }

func (f *form) setFocus(i int) tea.Cmd {
	f.inputs[f.active[f.focus]].Blur()
	f.focus = (i + len(f.active)) % len(f.active)
	return f.inputs[f.active[f.focus]].Focus()
}

// update handles navigation keys; it reports submit/cancel via the bools.
func (f form) update(msg tea.Msg) (form, tea.Cmd, bool, bool) {
	if k, ok := msg.(tea.KeyPressMsg); ok && f.discard {
		f.discard = false
		switch k.String() {
		case "y":
			return f, nil, false, true
		case "s", "ctrl+s":
			return f, nil, true, false
		}
		return f, nil, false, false // anything else: keep editing
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			if f.dirty() {
				f.discard = true
				return f, nil, false, false
			}
			return f, nil, false, true
		case "ctrl+s":
			return f, nil, true, false
		case "enter":
			if f.focus == len(f.active)-1 {
				return f, nil, true, false
			}
			return f, f.setFocus(f.focus + 1), false, false
		case "tab", "down":
			return f, f.setFocus(f.focus + 1), false, false
		case "shift+tab", "up":
			return f, f.setFocus(f.focus - 1), false, false
		case "left", "right":
			if i := f.active[f.focus]; isColorField(i) {
				dir := 1
				if k.String() == "left" {
					dir = -1
				}
				f.inputs[i].SetValue(pickStep(f.inputs[i].Value(), dir))
				f.inputs[i].CursorEnd()
				return f, nil, false, false
			}
		}
	}
	var cmd tea.Cmd
	i := f.active[f.focus]
	f.inputs[i], cmd = f.inputs[i].Update(msg)
	return f, cmd, false, false
}

func (f form) view() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(f.title) + "\n\n")
	for pos, i := range f.active {
		label := labelStyle.Render(f.label(i))
		marker := "  "
		if pos == f.focus {
			marker = lipgloss.NewStyle().Foreground(accent).Render("▸ ")
		}
		switch {
		case isColorField(i) && pos == f.focus:
			lines := strings.Split(pickerStrip(f.inputs[i].Value()), "\n")
			b.WriteString(marker + label + " " + lines[0] + "\n")
			b.WriteString(strings.Repeat(" ", 13) + lines[1] + "\n")
			b.WriteString(strings.Repeat(" ", 13) + f.inputs[i].View() + "\n")
		case isColorField(i):
			b.WriteString(marker + label + " " + pickerValue(f.inputs[i].Value()) + "\n")
		default:
			b.WriteString(marker + label + " " + f.inputs[i].View() + "\n")
		}
	}
	if c, fl := f.inputs[fColor].Value(), f.inputs[fFill].Value(); c != "" || fl != "" {
		if st, ok := mark(c, fl); ok {
			name := strings.TrimSpace(f.inputs[fName].Value())
			if name == "" {
				name = "…"
			}
			b.WriteString("\n" + strings.Repeat(" ", 13) + mutedStyle.Render(T("Preview:")+" ") + chip(st, name, markColor(fl) != nil) + "\n")
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
	if f.folderMode {
		b.WriteString(mutedStyle.Render(T("User, port, jump and key apply to every session inside.")) + "\n")
	}
	b.WriteString(mutedStyle.Render(T("tab/↑↓ move · ⏎ next · ctrl+s save · esc cancel")))
	return b.String()
}
