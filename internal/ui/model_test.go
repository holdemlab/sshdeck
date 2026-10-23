package ui

import (
	"image/color"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/crypto/ssh"

	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/i18n"
	"github.com/holdemlab/sshdeck/internal/localshell"
	"github.com/holdemlab/sshdeck/internal/sshtest"
)

func setup(t *testing.T, sessions string) (Model, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.yaml")
	os.WriteFile(path, []byte(sessions), 0o600)
	s := config.DefaultSettings()
	s.KnownHosts = filepath.Join(dir, "known_hosts")
	s.UseAgent = false
	s.Identities = nil
	s.Keepalive = 0
	s.LocalShell = false // tests that need the local entry turn it on
	m, err := New(Options{ConfigPath: path, SettingsPath: filepath.Join(dir, "config.yaml"), Settings: s})
	if err != nil {
		t.Fatal(err)
	}
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, path
}

// send feeds messages through Update. Commands (cursor blink etc.) are not
// executed.
func send(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func key(k string) tea.Msg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	case "ctrl+a":
		return tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

func keys(ks ...string) []tea.Msg {
	var out []tea.Msg
	for _, k := range ks {
		out = append(out, key(k))
	}
	return out
}

func text(s string) []tea.Msg {
	var out []tea.Msg
	for _, r := range s {
		out = append(out, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return out
}

// closeAll closes every tab and waits for the connection goroutines to end,
// so nothing writes into the test's temp dir afterwards.
func closeAll(t *testing.T, m Model) {
	for _, tab := range m.tabs {
		tab.close()
	}
	deadline := time.Now().Add(5 * time.Second)
	for _, tab := range m.tabs {
		for tab.live() && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func waitScreen(t *testing.T, tab *Tab, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(tab.screenText(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("screen never showed %q:\n%s", want, tab.screenText())
}

const treeYAML = `
folders:
  - name: prod
    jump: bastion.example.com
    sessions:
      - {name: web-01, host: 10.0.0.1}
      - {name: db-01, host: 10.0.0.9}
sessions:
  - {name: home, host: home.lan, user: me}
`

func TestTreeNavigation(t *testing.T) {
	m, _ := setup(t, treeYAML)
	if len(m.rows) != 4 {
		t.Fatalf("rows = %d", len(m.rows))
	}
	view := m.View().Content
	for _, want := range []string{"sshdeck", "Sessions", "prod", "web-01", "home", "Pick a session"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}
	m = send(m, keys("j")...)
	if !strings.Contains(ansi.Strip(m.View().Content), "via bastion.example.com") {
		t.Error("details do not show the inherited jump host")
	}
	m = send(m, keys("k", "h")...) // collapse prod
	if len(m.rows) != 2 {
		t.Errorf("after collapse rows = %d", len(m.rows))
	}
	m = send(m, keys("/")...)
	m = send(m, text("db")...)
	if len(m.rows) == 0 || m.rows[0].name != "db-01" {
		t.Errorf("filter rows = %+v", m.rows)
	}
}

func TestFormSavesSession(t *testing.T) {
	m, path := setup(t, treeYAML)
	m = send(m, keys("G", "n")...) // on root "home" → empty folder
	if m.side != sideForm {
		t.Fatal("form not open")
	}
	msgs := text("api-1")
	msgs = append(msgs, key("tab"))
	msgs = append(msgs, text("staging")...)
	msgs = append(msgs, key("tab"))
	msgs = append(msgs, text("api.example.com")...)
	msgs = append(msgs, key("ctrl+s"))
	m = send(m, msgs...)
	if m.side != sideBrowse {
		t.Fatalf("form still open: %q", m.form.err)
	}
	c, _ := config.Load(path)
	r, ok := c.Find("staging/api-1")
	if !ok || r.Host != "api.example.com" {
		t.Fatalf("saved = %+v %v", r, ok)
	}
	if n, _ := m.current(); n.name != "api-1" {
		t.Errorf("cursor on %q", n.name)
	}

	m = send(m, keys("d", "y")...)
	c, _ = config.Load(path)
	if _, ok := c.Find("staging/api-1"); ok {
		t.Error("delete did not remove the session")
	}
}

func TestTabConnectsAndTypes(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"alice": "Zq-9!pass"}})
	host, port, _ := net.SplitHostPort(srv.Addr)
	m, _ := setup(t, "sessions:\n  - {name: echo, host: "+host+", port: "+port+", user: alice}\n")

	m = send(m, keys("enter")...)
	if len(m.tabs) != 1 || m.focus != focusTerm {
		t.Fatalf("tabs=%d focus=%v", len(m.tabs), m.focus)
	}
	tab := m.tabs[0]
	waitScreen(t, tab, "continue connecting (yes/no)?")
	m = send(m, text("yes")...)
	m = send(m, keys("enter")...)
	waitScreen(t, tab, "password:")
	m = send(m, text("Zq-9!pass")...)
	m = send(m, keys("enter")...)
	waitScreen(t, tab, "welcome xterm-256color")
	if strings.Contains(tab.screenText(), "Zq-9!pass") {
		t.Error("password was echoed")
	}
	if st, _ := tab.getState(); st != stateConnected {
		t.Fatalf("state = %v", st)
	}
	// the pty matches the terminal area: 100 wide minus the sidebar, 30-2 high
	want := strconv.Itoa(100-m.opts.Settings.SidebarWidth) + "x28"
	waitScreen(t, tab, want)

	m = send(m, text("hello")...)
	m = send(m, keys("enter")...)
	waitScreen(t, tab, "you said: hello")

	// prefix commands: s → sidebar, then tab back to the terminal
	m = send(m, keys("ctrl+a", "s")...)
	if m.focus != focusSidebar {
		t.Error("ctrl+a s did not focus the sidebar")
	}
	m = send(m, keys("tab")...)
	if m.focus != focusTerm {
		t.Error("tab did not return to the terminal")
	}

	// duplicate, then close the duplicate with confirmation
	m = send(m, keys("ctrl+a", "d")...)
	if len(m.tabs) != 2 || m.active != 1 {
		t.Fatalf("after duplicate tabs=%d active=%d", len(m.tabs), m.active)
	}
	m = send(m, keys("ctrl+a", "w")...)
	if m.overlay != overlayConfirmClose {
		t.Fatal("close did not ask for confirmation")
	}
	m = send(m, keys("y")...)
	if len(m.tabs) != 1 || m.active != 0 {
		t.Fatalf("after close tabs=%d active=%d", len(m.tabs), m.active)
	}

	// remote exit → closed state, r reconnects
	m = send(m, text("exit")...)
	m = send(m, keys("enter")...)
	waitScreen(t, tab, "Press r to reconnect")
	m = send(m, keys("r")...)
	waitScreen(t, tab, "password:")
	closeAll(t, m)
}

func TestQuitAsksWithLiveTabs(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"a": "b"}})
	host, port, _ := net.SplitHostPort(srv.Addr)
	m, _ := setup(t, "sessions:\n  - {name: s, host: "+host+", port: "+port+", user: a}\n")
	m = send(m, keys("enter")...)
	m = send(m, keys("ctrl+a", "q")...)
	if m.overlay != overlayConfirmQuit {
		t.Fatal("quit did not ask for confirmation")
	}
	m = send(m, keys("n")...)
	if m.overlay != overlayNone || len(m.tabs) != 1 {
		t.Fatal("cancelled quit changed state")
	}
	closeAll(t, m)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTunnelsPanel(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"u": "pw"}})
	svc, _ := net.Listen("tcp", "127.0.0.1:0") // the "closed" service
	defer svc.Close()
	go func() {
		for {
			c, err := svc.Accept()
			if err != nil {
				return
			}
			c.Write([]byte("hi from service\n"))
			c.Close()
		}
	}()
	host, port, _ := net.SplitHostPort(srv.Addr)
	m, path := setup(t, "sessions:\n  - name: s\n    host: "+host+"\n    port: "+port+"\n    user: u\n    forwards: [\"L 0:"+svc.Addr().String()+"\"]\n")

	m = send(m, keys("t")...) // tunnels-only tab
	tab := m.tabs[0]
	if !tab.tunnelOnly || !strings.Contains(tab.title(), "⇄") {
		t.Fatal("t did not open a tunnels-only tab")
	}
	waitScreen(t, tab, "(yes/no)?")
	m = send(m, text("yes")...)
	m = send(m, keys("enter")...)
	waitScreen(t, tab, "password:")
	m = send(m, text("pw")...)
	m = send(m, keys("enter")...)
	waitScreen(t, tab, "Tunnels only")
	waitFor(t, "configured forward", func() bool { return tab.listening() == 1 })

	// the configured forward reaches the service
	addr := tab.forwardList()[0].Stats().Addr
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, _ := c.Read(buf)
	c.Close()
	if string(buf[:n]) != "hi from service\n" {
		t.Fatalf("through tunnel: %q", buf[:n])
	}

	// panel: add a SOCKS forward, toggle the first one off, save
	m = send(m, keys("ctrl+a", "t")...)
	if m.overlay != overlayTunnels {
		t.Fatal("ctrl+a t did not open the panel")
	}
	m = send(m, keys("a")...)
	m = send(m, text("D 0")...)
	m = send(m, keys("enter")...)
	if len(tab.forwardList()) != 2 || m.tunErr != "" {
		t.Fatalf("add failed: %q", m.tunErr)
	}
	waitFor(t, "two forwards", func() bool { return tab.listening() == 2 })
	if !strings.Contains(ansi.Strip(m.View().Content), "(SOCKS5)") {
		t.Error("panel does not list the SOCKS forward")
	}
	m = send(m, keys("k", "space")...)
	if tab.listening() != 1 {
		t.Errorf("after toggle listening = %d", tab.listening())
	}
	m = send(m, keys("w")...)
	c2, _ := config.Load(path)
	if got := c2.Session(nil, "s").Forwards; len(got) != 2 || got[1] != "D 0" {
		t.Errorf("saved forwards = %q", got)
	}

	// bad spec is reported, not added
	m = send(m, keys("a")...)
	m = send(m, text("L nonsense")...)
	m = send(m, keys("enter")...)
	if m.tunErr == "" || len(tab.forwardList()) != 2 {
		t.Errorf("bad spec: err=%q forwards=%d", m.tunErr, len(tab.forwardList()))
	}
	// typed text + esc asks before throwing it away
	m = send(m, keys("esc")...)
	if !m.tunAsk || !strings.Contains(ansi.Strip(m.View().Content), "Discard this tunnel?") {
		t.Fatal("esc on a typed tunnel did not ask")
	}
	m = send(m, keys("y")...)
	if m.tunAdding || len(tab.forwardList()) != 2 {
		t.Errorf("discard: adding=%v forwards=%d", m.tunAdding, len(tab.forwardList()))
	}
	closeAll(t, m)
	if tab.listening() != 0 {
		t.Error("forwards still listening after close")
	}
}

func TestSettingsScreen(t *testing.T) {
	m, _ := setup(t, treeYAML)
	m.opts.SettingsPath = filepath.Join(t.TempDir(), "config.yaml")

	m = send(m, keys("ctrl+a", ",")...)
	if !m.settingsOpen {
		t.Fatal("ctrl+a , did not open settings")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Prefix key") {
		t.Error("settings not rendered")
	}
	// prefix: enter, then the new key (ctrl+a itself must not be swallowed)
	m = send(m, keys("tab", "tab", "enter")...)
	m = send(m, tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl})
	if got := m.settings.fields[sPrefix].key; got != "ctrl+]" {
		t.Fatalf("captured prefix = %q", got)
	}
	// a plain letter is refused as prefix
	m = send(m, keys("enter", "x")...)
	if m.settings.fields[sPrefix].key != "ctrl+]" || m.settings.err == "" {
		t.Error("plain key accepted as prefix")
	}
	// sidebar width: clear and type 40
	m = send(m, keys("tab")...)
	m = send(m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = send(m, text("40")...)
	// toggle confirm close (last field, wrapping backwards) with space
	m = send(m, keys("shift+tab", "shift+tab", "shift+tab", "shift+tab")...)
	if m.settings.focus != sConfirm {
		t.Fatalf("focus = %d", m.settings.focus)
	}
	m = send(m, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m = send(m, keys("ctrl+s")...)
	if m.settingsOpen {
		t.Fatalf("save failed: %q", m.settings.err)
	}
	s := m.opts.Settings
	if s.Prefix != "ctrl+]" || s.SidebarWidth != 40 || s.ConfirmClose {
		t.Errorf("applied settings = %+v", s)
	}
	if m.termRect().Min.X != 40 {
		t.Errorf("sidebar width not applied: %d", m.termRect().Min.X)
	}
	saved, err := config.LoadSettings(m.opts.SettingsPath, false)
	if err != nil || saved.Prefix != "ctrl+]" || saved.SidebarWidth != 40 {
		t.Errorf("saved = %+v, %v", saved, err)
	}
	// the new prefix works, the old one is now an ordinary key
	m = send(m, tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl})
	if !m.prefixArmed {
		t.Error("new prefix not active")
	}
	m = send(m, keys("esc", "ctrl+a")...)
	if m.prefixArmed {
		t.Error("old prefix still active")
	}

	// invalid values keep the screen open with an error
	m = send(m, tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl}, key(","))
	m = send(m, keys("tab", "tab", "tab", "tab")...) // scrollback
	m = send(m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = send(m, text("lots")...)
	m = send(m, keys("ctrl+s")...)
	if !m.settingsOpen || !strings.Contains(m.settings.err, "Scrollback") {
		t.Errorf("bad scrollback: open=%v err=%q", m.settingsOpen, m.settings.err)
	}
	// out-of-range sidebar is refused too
	m = send(m, keys("shift+tab")...)
	m = send(m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = send(m, text("2632")...)
	m = send(m, keys("ctrl+s")...)
	if !m.settingsOpen || !strings.Contains(m.settings.err, "Sidebar") {
		t.Errorf("sidebar 2632: open=%v err=%q", m.settingsOpen, m.settings.err)
	}
	m = send(m, keys("esc")...)
	if !m.settingsOpen || !m.settings.discard {
		t.Fatal("esc with changes closed settings without asking")
	}
	m = send(m, keys("y")...)
	if m.settingsOpen || m.opts.Settings.Scrollback != config.DefaultSettings().Scrollback {
		t.Error("esc did not discard changes")
	}
}

func TestEditorReload(t *testing.T) {
	m, sessionsPath := setup(t, treeYAML)
	dir := t.TempDir()
	m.opts.SettingsPath = filepath.Join(dir, "config.yaml")

	// a fake editor that appends a session / breaks the settings file
	script := filepath.Join(dir, "ed.sh")
	os.WriteFile(script, []byte("#!/bin/sh\ncase \"$1\" in\n*sessions*) printf '  - {name: new-one, host: n.example}\\n' >> \"$1\";;\n*) printf 'prefix: x\\n' > \"$1\";;\nesac\n"), 0o755)
	t.Setenv("VISUAL", script)

	if err := editorCommand(sessionsPath).Run(); err != nil {
		t.Fatal(err)
	}
	m = m.editorDone(editorDoneMsg{target: editSessions})
	if _, ok := m.cfg.Find("new-one"); !ok || !m.statusOK {
		t.Errorf("sessions not reloaded: %q", m.status)
	}

	before := m.opts.Settings
	if err := editorCommand(m.opts.SettingsPath).Run(); err != nil {
		t.Fatal(err)
	}
	m = m.editorDone(editorDoneMsg{target: editSettings})
	if m.statusOK || !strings.Contains(m.status, "previous settings kept") || m.opts.Settings.Prefix != before.Prefix {
		t.Errorf("broken config applied: status %q prefix %q", m.status, m.opts.Settings.Prefix)
	}
}

const folderYAML = `
folders:
  - name: infra
    sessions:
      - {name: proxy, host: p.example.com}
  - name: prod
    jump: infra/proxy
    sessions:
      - {name: web-01, host: 10.0.0.1}
      - {name: db-01, host: 10.0.0.9}
sessions:
  - {name: home, host: home.lan}
`

func selectKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	for i, n := range m.rows {
		if n.key == key {
			m.cursor = i
			return m
		}
	}
	t.Fatalf("row %s not found", key)
	return m
}

func clearAndType(s string) []tea.Msg {
	return append([]tea.Msg{tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}}, text(s)...)
}

func TestEditFolder(t *testing.T) {
	m, path := setup(t, folderYAML)
	m = selectKey(t, m, "f:infra")
	m = send(m, keys("e")...)
	if m.side != sideForm || !m.form.folderMode {
		t.Fatal("e on a folder did not open the folder form")
	}
	// rename infra → core and give it a default user
	m = send(m, clearAndType("core")...)
	m = send(m, keys("tab", "tab")...) // Inside → User
	m = send(m, text("ops")...)
	m = send(m, keys("ctrl+s")...)
	if m.side != sideBrowse {
		t.Fatalf("save failed: %q", m.form.err)
	}
	c, _ := config.Load(path)
	if f := c.FolderAt([]string{"core"}); f == nil || f.User != "ops" {
		t.Fatalf("folder not renamed/updated: %+v", c.Folders)
	}
	if got := c.FolderAt([]string{"prod"}).Jump; got != "core/proxy" {
		t.Errorf("jump reference not rewritten: %q", got)
	}
	if n, _ := m.current(); n.key != "f:core" {
		t.Errorf("cursor on %q", n.key)
	}
	// the inherited user shows up for the proxy session
	r, _ := c.Find("core/proxy")
	if r.User != "ops" {
		t.Errorf("resolved user = %q", r.User)
	}
}

func TestDeleteFolderWithConfirm(t *testing.T) {
	m, path := setup(t, folderYAML)
	m = selectKey(t, m, "f:infra")
	m = send(m, keys("d")...)
	if m.overlay != overlayConfirmDelete {
		t.Fatal("no confirmation for folder delete")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Delete folder infra?") || !strings.Contains(view, "1 session(s)") || !strings.Contains(view, "jump") {
		t.Errorf("confirmation text:\n%s", view)
	}
	m = send(m, keys("n")...) // cancel
	if c, _ := config.Load(path); c.FolderAt([]string{"infra"}) == nil {
		t.Fatal("cancelled delete removed the folder")
	}
	m = send(m, keys("d", "y")...)
	if c, _ := config.Load(path); c.FolderAt([]string{"infra"}) != nil {
		t.Fatal("folder not deleted")
	}
	if !strings.Contains(m.status, "deleted infra") {
		t.Errorf("status = %q", m.status)
	}
}

func TestCopyAndNewFolder(t *testing.T) {
	m, path := setup(t, folderYAML)
	m = selectKey(t, m, "s:prod/web-01")
	m = send(m, keys("c")...)
	if m.form.editing != nil || m.form.inputs[fName].Value() != "web-01-copy" {
		t.Fatalf("copy form: editing=%v name=%q", m.form.editing, m.form.inputs[fName].Value())
	}
	m = send(m, keys("ctrl+s")...)
	c, _ := config.Load(path)
	if r, ok := c.Find("prod/web-01-copy"); !ok || r.Host != "10.0.0.1" {
		t.Fatalf("copy not saved: %+v %v", r, ok)
	}
	if _, ok := c.Find("prod/web-01"); !ok {
		t.Fatal("original disappeared")
	}

	m = send(m, keys("N")...)
	if m.inline.mode != inlineNewFolder || strings.Join(m.inline.parent, "/") != "prod" {
		t.Fatalf("N: inline=%+v", m.inline)
	}
	m = send(m, text("blue")...)
	m = send(m, keys("enter")...)
	c, _ = config.Load(path)
	if c.FolderAt([]string{"prod", "blue"}) == nil {
		t.Errorf("folder not created: %q", m.status)
	}
}

func TestEditTabSessionAndRename(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"u": "pw"}})
	host, port, _ := net.SplitHostPort(srv.Addr)
	m, path := setup(t, "sessions:\n  - {name: s, host: "+host+", port: "+port+", user: u}\n  - {name: via, host: x, jump: s}\n")
	m = send(m, keys("enter")...)
	tab := m.tabs[0]
	if m.focus != focusTerm {
		t.Fatal("tab not focused")
	}
	// prefix e edits the tab's session even though the terminal has focus
	m = send(m, keys("ctrl+a", "e")...)
	if m.side != sideForm || m.form.editing == nil || m.form.editing.name != "s" {
		t.Fatalf("ctrl+a e: side=%v editing=%+v", m.side, m.form.editing)
	}
	m = send(m, clearAndType("renamed")...)
	m = send(m, keys("ctrl+s")...)
	if m.side != sideBrowse {
		t.Fatalf("save: %q", m.form.err)
	}
	c, _ := config.Load(path)
	if _, ok := c.Find("renamed"); !ok {
		t.Fatal("session not renamed")
	}
	if got := c.Session(nil, "via").Jump; got != "renamed" {
		t.Errorf("jump ref = %q", got)
	}
	if tab.session.Name != "renamed" {
		t.Errorf("open tab still has %q", tab.session.Name)
	}
	closeAll(t, m)
}

func TestQuickFolderAndMove(t *testing.T) {
	m, path := setup(t, folderYAML)
	load := func() *config.Config { c, _ := config.Load(path); return c }

	// nested folders in one go, at top level (cursor on the top-level session)
	m = selectKey(t, m, "s:home")
	m = send(m, keys("N")...)
	if !strings.Contains(ansi.Strip(m.View().Content), "+ ") {
		t.Error("inline input not shown in the tree")
	}
	m = send(m, text("lab/vms/test")...)
	m = send(m, keys("enter")...)
	if load().FolderAt([]string{"lab", "vms", "test"}) == nil {
		t.Fatalf("nested folders not created: %q", m.status)
	}
	if n, _ := m.current(); n.key != "f:lab/vms/test" {
		t.Errorf("cursor on %q", n.key)
	}
	// duplicate name: error, input stays open
	m = selectKey(t, m, "s:home")
	m = send(m, keys("N")...)
	m = send(m, text("lab")...)
	m = send(m, keys("enter")...)
	if m.inline.mode != inlineNewFolder || m.statusOK {
		t.Errorf("duplicate accepted: %q", m.status)
	}
	m = send(m, keys("esc", "y")...) // typed text: esc asks, y discards
	if m.inline.mode != inlineNone {
		t.Fatal("inline input still open")
	}

	// cut a session, paste it into lab/vms
	m = selectKey(t, m, "s:prod/db-01")
	m = send(m, keys("x")...)
	if !strings.Contains(ansi.Strip(m.View().Content), "✂") {
		t.Error("cut item not marked")
	}
	m = selectKey(t, m, "f:lab/vms")
	m = send(m, keys("p")...)
	c := load()
	if _, ok := c.Find("lab/vms/db-01"); !ok {
		t.Fatalf("session not moved: %q", m.status)
	}
	if _, ok := c.Find("prod/db-01"); ok {
		t.Error("session still at the old place")
	}
	if m.clip != nil {
		t.Error("clipboard not cleared")
	}

	// cut a folder, try pasting into itself, then to top level
	m = selectKey(t, m, "f:lab/vms")
	m = send(m, keys("x")...)
	m = selectKey(t, m, "f:lab/vms/test")
	m = send(m, keys("p")...)
	if m.statusOK || !strings.Contains(m.status, "into itself") {
		t.Errorf("paste into itself: %q", m.status)
	}
	m = send(m, keys("P")...)
	c = load()
	if _, ok := c.Find("vms/db-01"); !ok || c.FolderAt([]string{"lab", "vms"}) != nil {
		t.Errorf("folder not moved to top level: %q", m.status)
	}

	// esc cancels a pending move
	m = selectKey(t, m, "s:home")
	m = send(m, keys("x", "esc")...)
	if m.clip != nil {
		t.Error("esc did not cancel the move")
	}
}

func TestInlineRename(t *testing.T) {
	m, path := setup(t, folderYAML)
	m = selectKey(t, m, "f:infra")
	m = send(m, keys("R")...)
	if m.inline.mode != inlineRename || m.inline.input.Value() != "infra" {
		t.Fatalf("R: %+v", m.inline)
	}
	m = send(m, clearAndType("core")...)
	m = send(m, keys("enter")...)
	c, _ := config.Load(path)
	if c.FolderAt([]string{"core"}) == nil || c.FolderAt([]string{"prod"}).Jump != "core/proxy" {
		t.Fatalf("folder rename: %q", m.status)
	}
	if c.Folders[0].Name != "core" {
		t.Error("renamed folder changed position")
	}

	m = selectKey(t, m, "s:prod/web-01")
	m = send(m, keys("R")...)
	m = send(m, clearAndType("web-1")...)
	m = send(m, keys("enter")...)
	c, _ = config.Load(path)
	if _, ok := c.Find("prod/web-1"); !ok {
		t.Fatalf("session rename: %q", m.status)
	}
	m = selectKey(t, m, "s:prod/web-1")
	m = send(m, keys("R")...)
	m = send(m, clearAndType("a/b")...)
	m = send(m, keys("enter")...)
	if m.inline.mode != inlineRename || m.statusOK {
		t.Error("name with / accepted")
	}
}

func TestEscAsksBeforeDiscarding(t *testing.T) {
	m, path := setup(t, folderYAML)

	// untouched form: esc closes at once
	m = selectKey(t, m, "s:prod/web-01")
	m = send(m, keys("e", "esc")...)
	if m.side != sideBrowse {
		t.Fatal("esc on an unchanged form should just close it")
	}

	// changed form: esc asks; n keeps editing with the text intact
	m = send(m, keys("e")...)
	m = send(m, text("-x")...)
	m = send(m, keys("esc")...)
	if m.side != sideForm || !m.form.discard {
		t.Fatal("esc with changes did not ask")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Discard changes?") {
		t.Error("question not shown")
	}
	m = send(m, keys("n")...)
	if m.side != sideForm || m.form.discard || m.form.inputs[fName].Value() != "web-01-x" {
		t.Fatalf("n should keep editing: side=%v value=%q", m.side, m.form.inputs[fName].Value())
	}
	// esc again, then s saves instead of discarding
	m = send(m, keys("esc", "s")...)
	if c, _ := config.Load(path); c.Session([]string{"prod"}, "web-01-x") == nil {
		t.Fatalf("s did not save: side=%v err=%q", m.side, m.form.err)
	}
	// new session with typed text: esc, y discards
	m = send(m, keys("n")...)
	m = send(m, text("tmp")...)
	m = send(m, keys("esc", "y")...)
	if m.side != sideBrowse {
		t.Fatal("y did not discard")
	}
	if c, _ := config.Load(path); c.Session([]string{"prod"}, "tmp") != nil {
		t.Error("discarded session was saved")
	}

	// folder form
	m = selectKey(t, m, "f:infra")
	m = send(m, keys("e")...)
	m = send(m, text("2")...)
	m = send(m, keys("esc")...)
	if !m.form.discard {
		t.Error("folder form did not ask")
	}
	m = send(m, keys("y")...)

	// inline rename: unchanged closes, changed asks
	m = send(m, keys("R", "esc")...)
	if m.inline.mode != inlineNone {
		t.Error("unchanged rename did not close")
	}
	m = send(m, keys("R")...)
	m = send(m, text("z")...)
	m = send(m, keys("esc")...)
	if !m.inline.ask || m.inline.mode != inlineRename {
		t.Fatal("changed rename did not ask")
	}
	m = send(m, keys("n")...)
	if m.inline.mode != inlineRename || m.inline.input.Value() != "infraz" {
		t.Errorf("n lost the text: %q", m.inline.input.Value())
	}
	m = send(m, keys("esc", "y")...)
	if m.inline.mode != inlineNone {
		t.Error("y did not discard the rename")
	}
	if c, _ := config.Load(path); c.FolderAt([]string{"infra"}) == nil {
		t.Error("discarded rename was applied")
	}
}

func TestCopyPaste(t *testing.T) {
	m, path := setup(t, folderYAML)
	load := func() *config.Config { c, _ := config.Load(path); return c }

	// y + p copies a session; p again makes another copy
	m = selectKey(t, m, "s:prod/web-01")
	m = send(m, keys("y")...)
	if !strings.Contains(ansi.Strip(m.View().Content), "⧉") {
		t.Error("copied item not marked")
	}
	m = selectKey(t, m, "f:infra")
	m = send(m, keys("p", "p")...)
	c := load()
	if c.Session([]string{"infra"}, "web-01") == nil || c.Session([]string{"infra"}, "web-01-copy") == nil {
		t.Fatalf("session copies missing: %q", m.status)
	}
	if c.Session([]string{"prod"}, "web-01") == nil {
		t.Fatal("original was moved instead of copied")
	}
	if m.clip == nil {
		t.Error("clipboard cleared after a copy paste")
	}
	m = send(m, keys("esc")...)
	if m.clip != nil {
		t.Error("esc did not clear the copy clipboard")
	}

	// y + P copies a whole folder to the top level; jumps inside are kept
	// pointing to the original (they reference infra, outside prod)
	m = selectKey(t, m, "f:prod")
	m = send(m, keys("y", "P")...)
	c = load()
	if c.FolderAt([]string{"prod-copy"}) == nil {
		t.Fatalf("folder copy missing: %q", m.status)
	}
	if _, ok := c.Find("prod-copy/db-01"); !ok {
		t.Error("sessions not copied with the folder")
	}
	if n, _ := m.current(); n.key != "f:prod-copy" {
		t.Errorf("cursor on %q", n.key)
	}
	m = send(m, keys("esc")...)

	// c on a folder: copy next to it and rename at once
	m = selectKey(t, m, "f:infra")
	m = send(m, keys("c")...)
	if m.inline.mode != inlineRename || m.inline.input.Value() != "infra-copy" {
		t.Fatalf("c on folder: inline=%+v status=%q", m.inline.mode, m.status)
	}
	m = send(m, clearAndType("infra-stage")...)
	m = send(m, keys("enter")...)
	c = load()
	if c.FolderAt([]string{"infra-stage"}) == nil || c.FolderAt([]string{"infra"}) == nil {
		t.Fatalf("duplicate+rename: %q", m.status)
	}

	// c on a session: form with a free name
	m = selectKey(t, m, "s:prod/web-01")
	m = send(m, keys("c")...)
	if m.side != sideForm || m.form.editing != nil || m.form.inputs[fName].Value() != "web-01-copy" {
		t.Fatalf("c on session: side=%v name=%q", m.side, m.form.inputs[fName].Value())
	}
}

func TestCopyFolderWithInnerJump(t *testing.T) {
	m, path := setup(t, `
folders:
  - name: acme
    folders:
      - name: dev
        sessions:
          - {name: proxy, host: 203.0.113.10, user: deploy}
          - {name: main, host: 10.0.0.12, user: deploy, jump: acme/dev/proxy}
`)
	m = selectKey(t, m, "f:acme/dev")
	m = send(m, keys("c")...)
	m = send(m, clearAndType("stage")...)
	m = send(m, keys("enter")...)
	c, _ := config.Load(path)
	if got := c.Session([]string{"acme", "stage"}, "main").Jump; got != "acme/stage/proxy" {
		t.Errorf("copied main should jump through the copied proxy, got %q", got)
	}
	if got := c.Session([]string{"acme", "dev"}, "main").Jump; got != "acme/dev/proxy" {
		t.Errorf("original changed: %q", got)
	}
}

// sendRun is send, but also runs the returned command once and feeds its
// message back (for async results such as key installation).
func sendRun(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		select {
		case out := <-done:
			if out != nil {
				if _, isBlink := out.(interface{ Blink() }); !isBlink {
					next, _ = m.Update(out)
					m = next.(Model)
				}
			}
		case <-time.After(10 * time.Second):
			t.Fatal("command did not finish")
		}
	}
	return m
}

func shShell(home string) sshtest.ShellFunc {
	return func(ch ssh.Channel, p sshtest.Pty, command string, resize <-chan sshtest.Pty) uint32 {
		if command == "" {
			return sshtest.EchoShell(ch, p, command, resize)
		}
		cmd := exec.Command("sh", "-c", command)
		cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = ch, ch, ch.Stderr()
		if err := cmd.Run(); err != nil {
			return 1
		}
		return 0
	}
}

func TestPasswordRememberedOnReconnect(t *testing.T) {
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"u": "Zq-9!pass"}})
	host, port, _ := net.SplitHostPort(srv.Addr)
	m, _ := setup(t, "sessions:\n  - {name: s, host: "+host+", port: "+port+", user: u}\n")
	m.opts.Settings.RememberPasswords = true

	m = send(m, keys("enter")...)
	tab := m.tabs[0]
	waitScreen(t, tab, "(yes/no)?")
	m = send(m, text("yes")...)
	m = send(m, keys("enter")...)
	waitScreen(t, tab, "password:")
	m = send(m, text("Zq-9!pass")...)
	m = send(m, keys("enter")...)
	waitScreen(t, tab, "welcome")

	// a second tab to the same server does not ask
	m = send(m, keys("ctrl+a", "d")...)
	tab2 := m.tabs[1]
	waitScreen(t, tab2, "welcome")
	if strings.Contains(tab2.screenText(), "password:") || !strings.Contains(tab2.screenText(), "Using remembered password") {
		t.Errorf("second tab:\n%s", tab2.screenText())
	}

	// switched off: forgotten at once, asks again
	s := m.opts.Settings
	s.RememberPasswords = false
	m.applySettings(s)
	m = send(m, keys("ctrl+a", "d")...)
	waitScreen(t, m.tabs[2], "password:")
	closeAll(t, m)
}

func TestInstallKeyDialog(t *testing.T) {
	localHome := t.TempDir()
	t.Setenv("HOME", localHome) // where the new key is created
	remoteHome := t.TempDir()
	srv := sshtest.Start(t, &sshtest.Server{Passwords: map[string]string{"u": "Zq-9!pass"}, Shell: shShell(remoteHome)})
	host, port, _ := net.SplitHostPort(srv.Addr)
	m, path := setup(t, "sessions:\n  - {name: s, host: "+host+", port: "+port+", user: u}\n")

	// not connected yet: refused with a hint
	m = send(m, keys("enter")...)
	tab := m.tabs[0]
	m = send(m, keys("ctrl+a", "k")...)
	if m.overlay == overlayInstallKey {
		t.Fatal("dialog opened before the connection was up")
	}
	waitScreen(t, tab, "(yes/no)?")
	m = send(m, text("yes")...)
	m = send(m, keys("enter")...)
	waitScreen(t, tab, "password:")
	m = send(m, text("Zq-9!pass")...)
	m = send(m, keys("enter")...)
	waitScreen(t, tab, "welcome")

	// no key anywhere: the dialog offers to create one
	m = send(m, keys("ctrl+a", "k")...)
	if m.overlay != overlayInstallKey || m.key.stage != keyGenerate {
		t.Fatalf("overlay=%v stage=%v", m.overlay, m.key.stage)
	}
	if m.key.path != filepath.Join(localHome, ".ssh", "id_ed25519") {
		t.Errorf("new key path = %q", m.key.path)
	}
	// mismatching passphrases are refused
	m = send(m, text("a")...)
	m = send(m, keys("enter")...)
	m = send(m, text("b")...)
	m = send(m, keys("enter")...)
	if m.key.err == "" || m.key.stage != keyGenerate {
		t.Fatalf("mismatch accepted: %+v", m.key.err)
	}
	// matching (empty here: both cleared) → create & install
	m.key.inputs[0].SetValue("")
	m.key.inputs[1].SetValue("")
	m = sendRun(t, m, key("enter"))
	if m.key.stage != keyUse {
		t.Fatalf("after install: stage=%v err=%q", m.key.stage, m.key.err)
	}
	data, _ := os.ReadFile(filepath.Join(remoteHome, ".ssh", "authorized_keys"))
	if !strings.Contains(string(data), m.key.pk.Line) {
		t.Fatalf("authorized_keys = %q", data)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Key added") {
		t.Error("result not shown")
	}
	// y: the session now uses the new key
	m = send(m, keys("y")...)
	c, _ := config.Load(path)
	if got := c.Session(nil, "s").Identity; got != "~/.ssh/id_ed25519" {
		t.Errorf("session identity = %q", got)
	}
	if tab.session.Identity != "~/.ssh/id_ed25519" {
		t.Errorf("open tab not updated: %q", tab.session.Identity)
	}

	// second time: the key is found and reported as already installed
	m = send(m, keys("ctrl+a", "k")...)
	if m.key.stage != keyConfirm {
		t.Fatalf("existing key not found: stage=%v", m.key.stage)
	}
	m = sendRun(t, m, key("y"))
	if m.key.stage != keyDone || !strings.Contains(m.key.msg, "already") {
		t.Errorf("second install: stage=%v msg=%q err=%q", m.key.stage, m.key.msg, m.key.err)
	}
	closeAll(t, m)
}

const deepYAML = `
folders:
  - name: prod
    folders:
      - name: web
        sessions:
          - {name: web-01, host: w}
      - name: db
        sessions:
          - {name: db-01, host: d}
  - name: lab
    sessions:
      - {name: vm, host: v}
sessions:
  - {name: home, host: h}
`

func TestFoldsRememberedAcrossRuns(t *testing.T) {
	m, path := setup(t, deepYAML)
	statePath := filepath.Join(t.TempDir(), "state", "state.yaml")
	m.opts.StatePath = statePath

	m = selectKey(t, m, "f:prod/web")
	m = send(m, keys("h")...) // fold prod/web
	m = selectKey(t, m, "f:lab")
	m = send(m, keys("enter")...) // enter also folds
	st, err := config.LoadState(statePath)
	if err != nil || strings.Join(st.Collapsed, ",") != "lab,prod/web" {
		t.Fatalf("state = %+v %v", st, err)
	}

	// "restart": a new model reads the same files
	s := m.opts.Settings
	m2, err := New(Options{ConfigPath: path, Settings: s, StatePath: statePath})
	if err != nil {
		t.Fatal(err)
	}
	m2 = send(m2, tea.WindowSizeMsg{Width: 100, Height: 30})
	if !m2.collapsed["f:prod/web"] || !m2.collapsed["f:lab"] || m2.collapsed["f:prod"] {
		t.Fatalf("restored = %v", m2.collapsed)
	}
	for _, n := range m2.rows {
		if n.key == "s:prod/web/web-01" || n.key == "s:lab/vm" {
			t.Errorf("%s visible inside a folded folder", n.key)
		}
	}

	// unfolding is remembered too
	m2 = selectKey(t, m2, "f:lab")
	m2 = send(m2, keys("l")...)
	st, _ = config.LoadState(statePath)
	if strings.Join(st.Collapsed, ",") != "prod/web" {
		t.Errorf("after unfold: %v", st.Collapsed)
	}
}

func TestFoldAllUnfoldAll(t *testing.T) {
	m, _ := setup(t, deepYAML)
	m.opts.StatePath = filepath.Join(t.TempDir(), "state.yaml")
	m = selectKey(t, m, "s:prod/db/db-01")
	m = send(m, keys("H")...)
	// only top-level folders and the root session are visible
	var visible []string
	for _, n := range m.rows {
		visible = append(visible, n.key)
	}
	if strings.Join(visible, " ") != "f:prod f:lab s:home" {
		t.Errorf("after fold all: %v", visible)
	}
	if n, _ := m.current(); n.key != "f:prod" {
		t.Errorf("cursor on %q, want the top-level folder", n.key)
	}
	// nested folders are folded too, so unfolding prod shows web and db folded
	m = send(m, keys("l")...)
	if !m.collapsed["f:prod/web"] || !m.collapsed["f:prod/db"] {
		t.Error("subfolders were not folded")
	}
	m = send(m, keys("*")...)
	if len(m.collapsed) != 0 || len(m.rows) != 8 {
		t.Errorf("after unfold all: collapsed=%v rows=%d", m.collapsed, len(m.rows))
	}
	if n, _ := m.current(); n.key != "f:prod" {
		t.Errorf("cursor moved to %q", n.key)
	}
	m = send(m, keys("-")...)
	if !m.collapsed["f:lab"] {
		t.Error("- did not fold all")
	}
	m = send(m, keys("L")...)
	if len(m.collapsed) != 0 {
		t.Error("L did not unfold all")
	}
}

func TestFoldsFollowRenameDeleteCopy(t *testing.T) {
	m, _ := setup(t, deepYAML)
	statePath := filepath.Join(t.TempDir(), "state.yaml")
	m.opts.StatePath = statePath
	m = selectKey(t, m, "f:prod/web")
	m = send(m, keys("h")...)

	// rename the parent: the fold moves with it
	m = selectKey(t, m, "f:prod")
	m = send(m, keys("R")...)
	m = send(m, clearAndType("live")...)
	m = send(m, keys("enter")...)
	if !m.collapsed["f:live/web"] || m.collapsed["f:prod/web"] {
		t.Fatalf("after rename: %v", m.collapsed)
	}
	// copy: the copy folds like the original
	m = selectKey(t, m, "f:live")
	m = send(m, keys("y", "P", "esc")...)
	if !m.collapsed["f:live-copy/web"] {
		t.Errorf("copy fold state: %v", m.collapsed)
	}
	// delete: its folds are dropped from the state file
	m = selectKey(t, m, "f:live-copy")
	m = send(m, keys("d", "y")...)
	st, _ := config.LoadState(statePath)
	if strings.Join(st.Collapsed, ",") != "live/web" {
		t.Errorf("state after delete: %v", st.Collapsed)
	}
}

func TestNewTopLevelFolderWithSlash(t *testing.T) {
	m, path := setup(t, `
folders:
  - name: prod
    sessions:
      - {name: web, host: w}
  - name: lab
`)
	// only folders at the root: the cursor is always inside one
	m = selectKey(t, m, "s:prod/web")
	m = send(m, keys("N")...)
	if !strings.Contains(m.status, "New folder in prod") {
		t.Errorf("destination not shown: %q", m.status)
	}
	m = send(m, text("/infra/dev")...)
	m = send(m, keys("enter")...)
	c, _ := config.Load(path)
	if c.FolderAt([]string{"infra", "dev"}) == nil {
		t.Fatalf("top-level folder not created: %q", m.status)
	}
	if c.FolderAt([]string{"prod", "infra"}) != nil {
		t.Error("created inside prod instead")
	}
	// without the slash it still goes inside the selection
	m = selectKey(t, m, "f:lab")
	m = send(m, keys("N")...)
	m = send(m, text("vms")...)
	m = send(m, keys("enter")...)
	c, _ = config.Load(path)
	if c.FolderAt([]string{"lab", "vms"}) == nil {
		t.Error("relative folder not created inside lab")
	}
}

func TestHelpScreen(t *testing.T) {
	m, _ := setup(t, treeYAML)
	i18n.Set(i18n.UK)
	defer i18n.Set(i18n.EN)
	m = send(m, keys("?")...)
	if m.overlay != overlayHelp {
		t.Fatal("? did not open the help")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "sshdeck — довідка") || strings.Contains(view, "# sshdeck") {
		t.Errorf("help not rendered as Markdown:\n%s", view)
	}
	// search jumps to the match
	m = send(m, keys("/")...)
	m = send(m, text("тунелі вкладки")...)
	m = send(m, keys("enter")...)
	if len(m.help.matches) == 0 || m.help.vp.YOffset() == 0 {
		t.Errorf("search: matches=%v offset=%d", m.help.matches, m.help.vp.YOffset())
	}
	m = send(m, keys("esc")...)
	if m.overlay != overlayNone {
		t.Fatal("esc did not close the help")
	}

	// the configured prefix is shown instead of ctrl+a
	m.opts.Settings.Prefix = "ctrl+]"
	m = send(m, keys("ctrl+]", "?")...)
	got := strings.Join(m.help.plain, "\n")
	if !strings.Contains(got, "ctrl+] s") || strings.Contains(got, "ctrl+a s") {
		t.Error("help does not use the configured prefix")
	}
	// light terminals get the light theme (re-rendered)
	before := m.help.key
	m = send(m, tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	if m.darkBG || m.help.key == before {
		t.Error("background colour not applied to the help")
	}
}

// The interface and the help follow the language setting, switched live.
func TestLanguageSwitch(t *testing.T) {
	m, _ := setup(t, treeYAML)
	defer i18n.Set(i18n.EN)
	m.opts.SettingsPath = filepath.Join(t.TempDir(), "config.yaml")
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "Sessions") {
		t.Fatalf("default should be English:\n%s", v)
	}
	m = send(m, keys("ctrl+a", ",")...)
	if m.settings.focus != sLanguage {
		t.Fatalf("focus = %d", m.settings.focus)
	}
	m = send(m, keys("right")...) // auto → uk
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "● Українська") {
		t.Fatalf("language choice not shown:\n%s", v)
	}
	m = send(m, keys("ctrl+s")...)
	if m.settingsOpen || m.opts.Settings.Language != "uk" || i18n.Current() != i18n.UK {
		t.Fatalf("language not applied: open=%v lang=%q err=%q", m.settingsOpen, m.opts.Settings.Language, m.settings.err)
	}
	v := ansi.Strip(m.View().Content)
	if !strings.Contains(v, "Сесії") || !strings.Contains(m.status, "налаштування збережено") {
		t.Errorf("interface not Ukrainian (status %q):\n%s", m.status, v)
	}
	saved, _ := os.ReadFile(m.opts.SettingsPath)
	if !strings.Contains(string(saved), "language: uk") || !strings.Contains(string(saved), "# Налаштування sshdeck") {
		t.Errorf("config.yaml:\n%s", saved)
	}
	m = send(m, keys("?")...)
	if !strings.Contains(strings.Join(m.help.plain, "\n"), "довідка") {
		t.Error("help is not Ukrainian")
	}
	m = send(m, keys("esc", "ctrl+a", ",")...)
	m = send(m, keys("right", "ctrl+s")...) // uk → en
	if i18n.Current() != i18n.EN || !strings.Contains(ansi.Strip(m.View().Content), "Sessions") {
		t.Error("switching back to English failed")
	}
	m = send(m, keys("?")...)
	if !strings.Contains(strings.Join(m.help.plain, "\n"), "help") {
		t.Error("help did not switch back to English")
	}
	// a broken value in the file is reported
	var st config.Settings = config.DefaultSettings()
	st.Language = "fr"
	if err := config.SaveSettings(m.opts.SettingsPath, st); err == nil {
		t.Error("language fr accepted")
	}
}

// Catppuccin themes recolor the whole frame; the settings screen previews them.
func TestThemes(t *testing.T) {
	m, _ := setup(t, treeYAML)
	defer useTheme("terminal")
	m.opts.SettingsPath = filepath.Join(t.TempDir(), "config.yaml")
	cellAt := func(m Model, x, y int) *uv.Cell {
		c := lipgloss.NewCanvas(m.width, m.height)
		uv.NewStyledString(m.View().Content).Draw(c, c.Bounds())
		return c.CellAt(x, y)
	}
	if c := cellAt(m, m.width-1, m.height/2); c.Style.Bg != nil {
		t.Fatalf("terminal theme painted the background: %v", c.Style.Bg)
	}
	for _, name := range []string{"latte", "frappe", "macchiato", "mocha"} {
		th := makeTheme(name)
		m.opts.Settings.Theme = name
		c := cellAt(m, m.width-1, m.height/2)
		if !sameColor(c.Style.Bg, th.bg) {
			t.Errorf("%s: background %v, want %v", name, c.Style.Bg, th.bg)
		}
		if _, err := th.glamourStyle(); err != nil {
			t.Errorf("%s: help style: %v", name, err)
		}
		// ANSI red from a remote program becomes the flavour's red
		if !sameColor(th.mapColor(ansi.BasicColor(1), nil), lipgloss.Color(th.p.Red)) {
			t.Errorf("%s: red not mapped", name)
		}
	}
	// preview while choosing, revert on discard
	m.opts.Settings.Theme = "terminal"
	m = send(m, keys("ctrl+a", ",", "tab", "left")...) // terminal → mocha
	if m.settings.previewTheme() != "mocha" {
		t.Fatalf("preview = %q", m.settings.previewTheme())
	}
	m.View()
	if themeName != "mocha" {
		t.Errorf("no preview: %s", themeName)
	}
	m = send(m, keys("esc", "y")...)
	m.View()
	if themeName != "terminal" {
		t.Errorf("discard kept the preview: %s", themeName)
	}
	// saved
	m = send(m, keys("ctrl+a", ",", "tab", "right", "ctrl+s")...) // terminal → latte
	saved, err := config.LoadSettings(m.opts.SettingsPath, false)
	if err != nil || saved.Theme != "latte" || m.opts.Settings.Theme != "latte" {
		t.Errorf("saved theme %q, %v", saved.Theme, err)
	}
	m = send(m, keys("?")...)
	if m.help.vp.View() == "" {
		t.Error("help not rendered with the theme")
	}
}

func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	r1, g1, b1, _ := a.RGBA()
	r2, g2, b2, _ := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2
}

// Folders and sessions can be marked with a text color and a fill; marks are
// inherited and show in the tree and on tabs.
func TestColorMarks(t *testing.T) {
	m, path := setup(t, treeYAML)
	// C on the prod folder: text red → right from "inherit" is "none", then rosewater…
	m = send(m, keys("C")...)
	if m.overlay != overlayColors {
		t.Fatal("C did not open the color dialog")
	}
	for m.colors.values[0] != "red" {
		m = send(m, keys("right")...)
	}
	m = send(m, keys("down")...)
	for m.colors.values[1] != "surface" {
		m = send(m, keys("left")...)
	}
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "Colors of prod") || !strings.Contains(v, "surface") {
		t.Errorf("dialog:\n%s", v)
	}
	m = send(m, keys("enter")...)
	if m.overlay != overlayNone || !m.statusOK {
		t.Fatalf("save: %q %q", m.colors.err, m.status)
	}
	cfg, _ := config.Load(path)
	if f := cfg.FolderAt([]string{"prod"}); f.Color != "red" || f.Fill != "surface" {
		t.Fatalf("saved %q/%q", f.Color, f.Fill)
	}
	// sessions inside inherit it; the row is drawn in the palette's red
	r, _ := cfg.Find("prod/web-01")
	if r.Color != "red" || r.Fill != "surface" {
		t.Errorf("not inherited: %+v", r.Defaults)
	}
	st, marked, filled := m.nodeMark(m.rows[1])
	if !marked || !filled || !sameColor(st.GetForeground(), lipgloss.Color(activePalette().Red)) {
		t.Error("session row not marked")
	}
	// "none" on a session stops the inheritance
	m = send(m, keys("j", "C", "right", "enter")...) // inherit → none
	cfg, _ = config.Load(path)
	if s := cfg.Session([]string{"prod"}, "web-01"); s.Color != "none" {
		t.Errorf("session color = %q", s.Color)
	}
	if st, _, filled := m.nodeMark(m.rows[1]); !filled || sameColor(st.GetForeground(), lipgloss.Color(activePalette().Red)) {
		t.Error("color none: the text should lose red and keep the inherited fill")
	}
	// the edit form: ← → picks, a typed #hex works, junk is refused
	m = send(m, keys("j", "e")...) // db-01
	for m.form.active[m.form.focus] != fColor {
		m = send(m, keys("tab")...)
	}
	m = send(m, keys("right", "right")...) // inherit → none → rosewater
	if got := m.form.inputs[fColor].Value(); got != "rosewater" {
		t.Errorf("picker = %q", got)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Preview:") {
		t.Error("no preview in the form")
	}
	m = send(m, keys("tab")...)
	m = send(m, text("#ABC")...)
	m = send(m, keys("ctrl+s")...)
	cfg, _ = config.Load(path)
	if s := cfg.Session([]string{"prod"}, "db-01"); s == nil || s.Color != "rosewater" || s.Fill != "#aabbcc" {
		t.Fatalf("form save: %+v (err %q)", s, m.form.err)
	}
	m = send(m, keys("e")...)
	for m.form.active[m.form.focus] != fFill {
		m = send(m, keys("tab")...)
	}
	m = send(m, text("zz")...)
	m = send(m, keys("ctrl+s")...)
	if m.side != sideForm || !strings.Contains(m.form.err, "not a color") {
		t.Errorf("junk color accepted: %q", m.form.err)
	}
}

// The build version shows at the right end of the tab bar and in the help.
func TestVersionShown(t *testing.T) {
	m, _ := setup(t, treeYAML)
	m.opts.Version = "v1.2.3"
	first := strings.Split(ansi.Strip(m.View().Content), "\n")[0]
	if !strings.HasSuffix(strings.TrimRight(first, " "), "v1.2.3") {
		t.Errorf("tab bar: %q", first)
	}
	m = send(m, keys("?")...)
	if !strings.Contains(ansi.Strip(m.View().Content), "sshdeck v1.2.3") {
		t.Error("version not in the help footer")
	}
	m = send(m, keys("esc")...)
	m = send(m, tea.WindowSizeMsg{Width: 12, Height: 20})
	if strings.Contains(strings.Split(ansi.Strip(m.View().Content), "\n")[0], "v1.2.3") {
		t.Error("version squeezed into a too narrow tab bar")
	}
}

// The local entry: first in the tree, opens a shell on this computer, and
// refuses the tree operations that make no sense for it.
func TestLocalShell(t *testing.T) {
	if !localshell.Supported() {
		t.Skip("no local shell here")
	}
	m, path := setup(t, treeYAML)
	s := m.opts.Settings
	s.LocalShell = true
	s.LocalCommand = `printf 'local:%s\n' "$SSHDECK"; read x; echo got:$x`
	m.applySettings(s)
	if len(m.rows) == 0 || m.rows[0].kind != localNode {
		t.Fatalf("local entry not first: %+v", m.rows)
	}
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "local this computer") {
		t.Errorf("tree:\n%s", v)
	}
	// built in: no rename / delete, sessions.yaml untouched
	before, _ := os.ReadFile(path)
	m = send(m, keys("g", "d")...)
	if m.overlay == overlayConfirmDelete || m.statusOK {
		t.Error("d on the local entry should be refused")
	}
	m = send(m, keys("R")...)
	if m.inline.mode != inlineNone {
		t.Error("R on the local entry should be refused")
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("sessions.yaml changed")
	}
	// enter opens a local tab running the command
	m = send(m, keys("enter")...)
	tab := m.activeTab()
	if tab == nil || !tab.local {
		t.Fatal("no local tab")
	}
	waitScreen(t, tab, "local:1")
	if !strings.Contains(tab.routeString(), "local: ") {
		t.Errorf("route %q", tab.routeString())
	}
	m = sendRun(t, m, key("o"))
	m = sendRun(t, m, key("k"))
	m = sendRun(t, m, key("enter"))
	waitScreen(t, tab, "got:ok")
	waitScreen(t, tab, "The shell has exited.")
	// tunnels and key install are refused on a local tab
	m = send(m, keys("ctrl+a", "t")...)
	if m.overlay == overlayTunnels || m.statusOK {
		t.Error("tunnels panel opened on a local tab")
	}
	// prefix + l opens another one; the setting hides the entry
	m = send(m, keys("ctrl+a", "l")...)
	if len(m.tabs) != 2 || !m.tabs[1].local {
		t.Errorf("ctrl+a l: %d tabs", len(m.tabs))
	}
	closeAll(t, m)
	s.LocalShell = false
	m.applySettings(s)
	if len(m.rows) > 0 && m.rows[0].kind == localNode {
		t.Error("local_shell: false still shows the entry")
	}
}

// The local entry takes colors through C; they are stored in config.yaml.
func TestLocalShellColors(t *testing.T) {
	if !localshell.Supported() {
		t.Skip("no local shell here")
	}
	m, path := setup(t, treeYAML)
	s := m.opts.Settings
	s.LocalShell = true
	m.applySettings(s)
	m = send(m, keys("g", "C", "down")...)
	for m.colors.values[1] != "green" {
		m = send(m, keys("right")...)
	}
	m = send(m, keys("enter")...)
	if m.opts.Settings.LocalFill != "green" {
		t.Fatalf("local fill = %q (%s)", m.opts.Settings.LocalFill, m.colors.err)
	}
	saved, err := config.LoadSettings(m.opts.SettingsPath, false)
	if err != nil || saved.LocalFill != "green" {
		t.Errorf("config.yaml: %q %v", saved.LocalFill, err)
	}
	cfg, _ := config.Load(path)
	if cfg.Fill != "" {
		t.Error("the local colors leaked into sessions.yaml")
	}
}

func click(x, y int) tea.Msg { return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft} }

// localTabs opens n local tabs running command.
func localTabs(t *testing.T, m Model, n int, command string) Model {
	t.Helper()
	if !localshell.Supported() {
		t.Skip("no local shell here")
	}
	s := m.opts.Settings
	s.LocalCommand = command
	m.applySettings(s)
	for range n {
		m.openLocalTab()
	}
	return m
}

// Mouse on the tab bar, the tree and the sidebar edge.
func TestMouseTabsTreeAndSidebar(t *testing.T) {
	m, _ := setup(t, treeYAML)
	m = localTabs(t, m, 2, "sleep 30")
	defer closeAll(t, m)
	spans := m.tabSpans()
	if len(spans) != 2 {
		t.Fatalf("spans %v", spans)
	}
	m = send(m, click(spans[0][0]+1, 0))
	if m.active != 0 {
		t.Errorf("click on tab 1: active = %d", m.active)
	}
	m = send(m, tea.MouseClickMsg{X: spans[1][0] + 1, Y: 0, Button: tea.MouseMiddle})
	if m.overlay != overlayConfirmClose || m.active != 1 {
		t.Errorf("middle click should ask to close tab 2: overlay %v active %d", m.overlay, m.active)
	}
	m = send(m, keys("n")...) // keep it

	// tree: rows start at y=3; prod (folder) is row 0
	m = send(m, click(20, 4)) // web-01
	if m.focus != focusSidebar || m.cursor != 1 {
		t.Errorf("click on a row: focus %v cursor %d", m.focus, m.cursor)
	}
	m = send(m, click(2, 3)) // the arrow of prod folds it
	if !m.collapsed["f:prod"] {
		t.Error("click on the arrow did not fold")
	}
	m = send(m, click(10, 3), click(10, 3)) // double click unfolds
	if m.collapsed["f:prod"] {
		t.Error("double click did not unfold")
	}
	m = send(m, tea.MouseWheelMsg{X: 10, Y: 5, Button: tea.MouseWheelDown})
	if m.cursor != 3 {
		t.Errorf("wheel: cursor %d", m.cursor)
	}

	// drag the sidebar edge to column 40 and release: saved
	sw := m.sidebarWidth()
	m = send(m, click(sw-1, 6), tea.MouseMotionMsg{X: 40, Y: 6, Button: tea.MouseLeft}, tea.MouseReleaseMsg{X: 40, Y: 6})
	if m.opts.Settings.SidebarWidth != 41 || m.termRect().Min.X != 41 {
		t.Errorf("drag: width %d", m.opts.Settings.SidebarWidth)
	}
	saved, err := config.LoadSettings(m.opts.SettingsPath, false)
	if err != nil || saved.SidebarWidth != 41 {
		t.Errorf("saved width %d %v", saved.SidebarWidth, err)
	}

	// mouse: false ignores everything
	s := m.opts.Settings
	s.Mouse = false
	m.applySettings(s)
	m = send(m, click(spans[0][0]+1, 0))
	if m.active != 1 {
		t.Error("mouse: false still switched tabs")
	}
	if m.View().MouseMode != tea.MouseModeNone {
		t.Error("mouse mode still requested")
	}
}

// Programs that ask for the mouse get it; otherwise the wheel scrolls.
func TestMouseInTerminal(t *testing.T) {
	m, _ := setup(t, treeYAML)
	m = localTabs(t, m, 1, `seq 1 100; printf 'ready\n'; read x; printf '\033[?1000h\033[?1006hmouse-on\n'; cat`)
	defer closeAll(t, m)
	tab := m.activeTab()
	waitScreen(t, tab, "ready")
	tr := m.termRect()

	// no mouse reporting yet: the wheel scrolls the history
	m = send(m, tea.MouseWheelMsg{X: tr.Min.X + 5, Y: tr.Min.Y + 5, Button: tea.MouseWheelUp})
	if !m.scrolling || tab.scroll != wheelLines {
		t.Fatalf("wheel up: scrolling %v scroll %d", m.scrolling, tab.scroll)
	}
	m = send(m, tea.MouseWheelMsg{X: tr.Min.X + 5, Y: tr.Min.Y + 5, Button: tea.MouseWheelDown})
	if m.scrolling || tab.scroll != 0 {
		t.Fatalf("wheel down: scrolling %v scroll %d", m.scrolling, tab.scroll)
	}

	// the program turns mouse reporting on: a click reaches it (the pty
	// echoes the SGR report as ^[[<0;6;3M)
	m = sendRun(t, m, key("enter"))
	waitScreen(t, tab, "mouse-on")
	deadline := time.Now().Add(5 * time.Second)
	for !tab.wantsMouse() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !tab.wantsMouse() {
		t.Fatal("mouse mode not noticed")
	}
	m = send(m, click(tr.Min.X+5, tr.Min.Y+2))
	waitScreen(t, tab, "[<0;6;3M")
	m = send(m, tea.MouseWheelMsg{X: tr.Min.X + 5, Y: tr.Min.Y + 2, Button: tea.MouseWheelUp})
	if m.scrolling {
		t.Error("the wheel scrolled instead of going to the program")
	}
	waitScreen(t, tab, "[<64;6;3M")
}

// Dragging in a tab selects text in the tab only and copies it.
func TestSelectionInTab(t *testing.T) {
	m, _ := setup(t, treeYAML)
	m = localTabs(t, m, 1, `printf 'alpha beta\ngamma delta\nready\n'; cat`)
	defer closeAll(t, m)
	tab := m.activeTab()
	waitScreen(t, tab, "ready")
	tr := m.termRect()
	at := func(x, y int) (int, int) { return tr.Min.X + x, tr.Min.Y + y }

	// from "beta" on line 0 to "gamma" on line 1
	x0, y0 := at(6, 0)
	x1, y1 := at(4, 1)
	m = send(m, tea.MouseClickMsg{X: x0, Y: y0, Button: tea.MouseLeft},
		tea.MouseMotionMsg{X: x1, Y: y1, Button: tea.MouseLeft},
		tea.MouseReleaseMsg{X: x1, Y: y1, Button: tea.MouseLeft})
	if m.lastCopy != "beta\ngamma" {
		t.Fatalf("copied %q", m.lastCopy)
	}
	if !m.statusOK || !strings.Contains(m.status, "copied 10") {
		t.Errorf("status %q", m.status)
	}
	// highlighted in the tab, nothing in the sidebar
	c := lipgloss.NewCanvas(m.width, m.height)
	uv.NewStyledString(m.View().Content).Draw(c, c.Bounds())
	if c.CellAt(x0, y0).Style.Attrs&uv.AttrReverse == 0 {
		t.Error("selection not highlighted")
	}

	// double click: the word
	xw, yw := at(1, 1)
	m = send(m, click(xw, yw), click(xw, yw))
	if m.lastCopy != "gamma" {
		t.Errorf("double click copied %q", m.lastCopy)
	}
	// triple click: the line
	m = send(m, click(xw, yw))
	if m.lastCopy != "gamma delta" {
		t.Errorf("triple click copied %q", m.lastCopy)
	}
	// middle click pastes it into the tab
	m = send(m, tea.MouseClickMsg{X: xw, Y: yw, Button: tea.MouseMiddle})
	m = sendRun(t, m, key("enter"))
	deadline := time.Now().Add(5 * time.Second) // printed, echoed, and cat's copy
	for strings.Count(tab.screenText(), "gamma delta") < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := strings.Count(tab.screenText(), "gamma delta"); n < 3 {
		t.Errorf("middle click did not paste (%d copies):\n%s", n, tab.screenText())
	}
	// typing clears the selection
	if m.sel.tab != 0 {
		t.Error("selection kept after typing")
	}
}

// Right click (or m) in the tree opens a context menu of the entry's actions.
func TestContextMenu(t *testing.T) {
	m, path := setup(t, treeYAML)
	// right click on web-01 (row 1, y = 4)
	m = send(m, tea.MouseClickMsg{X: 8, Y: 4, Button: tea.MouseRight})
	if m.overlay != overlayMenu || m.cursor != 1 {
		t.Fatalf("menu: overlay %v cursor %d", m.overlay, m.cursor)
	}
	for _, want := range []string{"Open", "Open tunnels only", "Edit…", "Rename", "Duplicate", "Copy", "Cut", "Delete…"} {
		if !strings.Contains(m.menuText(), want) {
			t.Errorf("session menu lacks %q:\n%s", want, m.menuText())
		}
	}
	if strings.Contains(m.menuText(), "Paste here") {
		t.Error("Paste offered with nothing to paste")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Open tunnels only") {
		t.Error("menu not drawn")
	}
	// the keyboard: down to "Open tunnels only"… then esc closes
	m = send(m, keys("down")...)
	if m.menu.items[m.menu.sel].key != "t" {
		t.Errorf("sel %d", m.menu.sel)
	}
	m = send(m, keys("esc")...)
	if m.overlay != overlayNone {
		t.Fatal("esc did not close the menu")
	}

	// a click on "Delete…" asks to delete web-01
	m = send(m, tea.MouseClickMsg{X: 8, Y: 4, Button: tea.MouseRight})
	r := m.menuRect()
	del := -1
	for i, it := range m.menu.items {
		if it.key == "d" {
			del = i
		}
	}
	m = send(m, click(r.Min.X+2, r.Min.Y+1+del))
	if m.overlay != overlayConfirmDelete || m.delNode.name != "web-01" {
		t.Fatalf("delete from the menu: overlay %v node %q", m.overlay, m.delNode.name)
	}
	m = send(m, keys("n")...)

	// m on a folder, then its letter: c duplicates it
	m = send(m, keys("g", "m")...)
	if m.overlay != overlayMenu || !strings.Contains(m.menuText(), "New folder here") || !strings.Contains(m.menuText(), "Collapse") {
		t.Fatalf("folder menu:\n%s", m.menuText())
	}
	m = send(m, keys("y")...) // Copy
	if m.clip == nil || !m.clip.copy {
		t.Fatal("y from the menu did not copy")
	}
	m = send(m, keys("esc")...) // drop the clipboard mode message
	m.clip = &clipboard{node: m.rows[0], copy: true}
	m = send(m, keys("m")...)
	if !strings.Contains(m.menuText(), "Paste here") {
		t.Error("Paste not offered with a copied entry")
	}
	m = send(m, keys("esc")...)

	// right click below the entries: top-level actions
	m = send(m, tea.MouseClickMsg{X: 8, Y: 15, Button: tea.MouseRight})
	if !m.menu.empty || !strings.Contains(m.menuText(), "New folder") || strings.Contains(m.menuText(), "Delete") {
		t.Fatalf("empty-area menu:\n%s", m.menuText())
	}
	m = send(m, keys("N")...)
	if m.inline.mode != inlineNewFolder || m.inline.parent != nil {
		t.Fatalf("new top-level folder: mode %v parent %v", m.inline.mode, m.inline.parent)
	}
	m = send(m, text("ops")...)
	m = send(m, keys("enter")...)
	cfg, _ := config.Load(path)
	if cfg.FolderAt([]string{"ops"}) == nil {
		t.Error("top-level folder not created")
	}
	// a click outside closes the menu without doing anything else
	m = send(m, tea.MouseClickMsg{X: 8, Y: 4, Button: tea.MouseRight})
	m = send(m, click(m.width-2, m.height-3))
	if m.overlay != overlayNone {
		t.Error("click outside did not close the menu")
	}
}
