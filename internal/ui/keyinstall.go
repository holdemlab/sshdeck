package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/sshclient"
)

// The install-key dialog (prefix + k) puts a local public key into
// ~/.ssh/authorized_keys on the server of the active tab, over the
// connection that is already open, like ssh-copy-id. Afterwards the session
// can log in with the key instead of a password.

type keyStage int

const (
	keyConfirm  keyStage = iota // a key was found: install it?
	keyGenerate                 // no key: create one (path + passphrase)
	keyWorking                  // installing
	keyUse                      // installed: use it for this session?
	keyDone                     // result / error, any key closes
)

type keyDialog struct {
	stage  keyStage
	tab    *Tab
	pk     sshclient.PublicKey
	path   string
	inputs [2]textinput.Model // passphrase, repeat
	focus  int
	msg    string
	err    string
}

type keyInstalledMsg struct {
	tabID int
	added bool
	err   error
}

func newPassInput(placeholder string) textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = placeholder
	in.EchoMode = textinput.EchoPassword
	in.SetWidth(32)
	return in
}

func (m *Model) openInstallKey() tea.Cmd {
	t := m.activeTab()
	if t == nil || t.connection() == nil {
		m.setStatus(false, "connect first: the key is installed over the open connection")
		return nil
	}
	m.key = keyDialog{tab: t}
	m.overlay = overlayInstallKey
	candidates := append([]string{t.session.Identity}, m.opts.Settings.Identities...)
	if pk, ok := sshclient.FindPublicKey(candidates...); ok {
		m.key.stage, m.key.pk = keyConfirm, pk
		return nil
	}
	return m.startGenerate()
}

func (m *Model) startGenerate() tea.Cmd {
	m.key.stage = keyGenerate
	m.key.path = sshclient.NewKeyPath()
	m.key.inputs = [2]textinput.Model{newPassInput(T("empty = no passphrase")), newPassInput(T("repeat"))}
	m.key.focus, m.key.err = 0, ""
	return m.key.inputs[0].Focus()
}

func (m Model) keyDialogKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	d := &m.key
	switch d.stage {
	case keyConfirm:
		switch k.String() {
		case "y", "enter":
			return m, m.install()
		case "g":
			return m, m.startGenerate()
		case "esc", "n", "q":
			m.overlay = overlayNone
		}
	case keyGenerate:
		switch k.String() {
		case "esc":
			m.overlay = overlayNone
			return m, nil
		case "tab", "down", "shift+tab", "up":
			d.inputs[d.focus].Blur()
			d.focus = 1 - d.focus
			return m, d.inputs[d.focus].Focus()
		case "enter":
			if d.focus == 0 {
				d.inputs[0].Blur()
				d.focus = 1
				return m, d.inputs[1].Focus()
			}
			pass := d.inputs[0].Value()
			if pass != d.inputs[1].Value() {
				d.err = T("passphrases do not match")
				d.inputs[1].SetValue("")
				return m, nil
			}
			pk, err := sshclient.GenerateKey(d.path, pass)
			if err != nil {
				d.err = err.Error()
				return m, nil
			}
			d.pk = pk
			return m, m.install()
		}
		var cmd tea.Cmd
		d.inputs[d.focus], cmd = d.inputs[d.focus].Update(k)
		return m, cmd
	case keyWorking:
		// wait for the result
	case keyUse:
		switch k.String() {
		case "y", "enter":
			if err := m.useKeyForSession(d.tab, d.pk.Path); err != nil {
				d.stage, d.err = keyDone, err.Error()
				return m, nil
			}
			m.overlay = overlayNone
			m.setStatus(true, "%s now logs in with %s", d.tab.session.ID(), tildePath(d.pk.Path))
		default:
			m.overlay = overlayNone
		}
	case keyDone:
		m.overlay = overlayNone
	}
	return m, nil
}

func (m *Model) install() tea.Cmd {
	d := &m.key
	conn := d.tab.connection()
	if conn == nil {
		d.stage, d.err = keyDone, T("the connection is closed — reconnect and try again")
		return nil
	}
	d.stage, d.err = keyWorking, ""
	pk, id := d.pk, d.tab.id
	return func() tea.Msg {
		added, err := conn.InstallKey(pk)
		return keyInstalledMsg{tabID: id, added: added, err: err}
	}
}

func (m Model) keyInstalled(msg keyInstalledMsg) Model {
	d := &m.key
	if m.overlay != overlayInstallKey || d.tab == nil || d.tab.id != msg.tabID {
		return m
	}
	if msg.err != nil {
		d.stage, d.err = keyDone, msg.err.Error()
		return m
	}
	if msg.added {
		d.msg = T("Key added to ~/.ssh/authorized_keys on %s.", d.tab.session.Host)
	} else {
		d.msg = T("This key was already in ~/.ssh/authorized_keys on %s.", d.tab.session.Host)
	}
	if config.ExpandHome(d.tab.session.Identity) != d.pk.Path {
		d.stage = keyUse
	} else {
		d.stage = keyDone
		d.msg += "\nThe session already uses this key: the next login needs no password."
	}
	return m
}

// useKeyForSession sets identity on the tab's session in sessions.yaml.
func (m *Model) useKeyForSession(t *Tab, keyPath string) error {
	cfg, err := config.Load(m.opts.ConfigPath)
	if err != nil {
		return err
	}
	s := cfg.Session(t.session.Path, t.session.Name)
	if s == nil {
		return fmt.Errorf("%s is no longer in %s", t.session.ID(), m.opts.ConfigPath)
	}
	s.Identity = tildePath(keyPath)
	if err := config.Save(m.opts.ConfigPath, cfg); err != nil {
		return err
	}
	m.cfg = cfg
	m.refreshRows()
	m.refreshTabSessions(func(id string) string { return id })
	return nil
}

func (m Model) renderKeyDialog() string {
	d := m.key
	var b strings.Builder
	target := T("this server")
	if d.tab != nil {
		target = d.tab.session.User + "@" + d.tab.session.Host
		if d.tab.session.User == "" {
			target = d.tab.session.Host
		}
	}
	b.WriteString(titleStyle.Render(T("Install public key on %s", target)) + "\n\n")
	switch d.stage {
	case keyConfirm:
		b.WriteString(T("Key:") + "  " + d.pk.Describe() + "\n\n")
		b.WriteString(mutedStyle.Render(T("It is appended to ~/.ssh/authorized_keys on the server\n(existing keys stay; nothing happens if it is already there).")) + "\n")
		if d.tab != nil && strings.Contains(d.tab.routeString(), "→") {
			b.WriteString(mutedStyle.Render(T("Only the target server is changed, not the jump hosts.")) + "\n")
		}
		b.WriteString("\n" + mutedStyle.Render(T("y install · g create a new key instead · esc cancel")))
	case keyGenerate:
		b.WriteString(T("No key to install yet — create a new ed25519 key:\n\n"))
		b.WriteString(labelStyle.Width(12).Render(T("Path")) + " " + tildePath(d.path) + "\n")
		marker := func(i int) string {
			if d.focus == i {
				return "▸ "
			}
			return "  "
		}
		b.WriteString(marker(0) + labelStyle.Width(10).Render(T("Passphrase")) + " " + d.inputs[0].View() + "\n")
		b.WriteString(marker(1) + labelStyle.Width(10).Render(T("Repeat")) + " " + d.inputs[1].View() + "\n\n")
		b.WriteString(mutedStyle.Render(T("A passphrase protects the key file; sshdeck asks for it once\nper run when remember_passwords is on.")) + "\n\n")
		if d.err != "" {
			b.WriteString(errStyle.Render("✗ "+d.err) + "\n")
		}
		b.WriteString(mutedStyle.Render(T("enter create & install · tab next field · esc cancel")))
	case keyWorking:
		b.WriteString(T("Installing %s …", d.pk.Describe()))
	case keyUse:
		b.WriteString(okStyle.Render("✓ "+d.msg) + "\n\n")
		b.WriteString(T("Use %s for %s from now on?", tildePath(d.pk.Path), d.tab.session.ID()) + "\n")
		b.WriteString(mutedStyle.Render(T("(sets the session's key file in sessions.yaml)")) + "\n\n")
		b.WriteString(mutedStyle.Render(T("y yes · any other key no")))
	case keyDone:
		if d.err != "" {
			b.WriteString(errStyle.Render("✗ "+d.err) + "\n\n")
		} else {
			b.WriteString(okStyle.Render("✓ "+d.msg) + "\n\n")
		}
		b.WriteString(mutedStyle.Render(T("press any key")))
	}
	return b.String()
}
