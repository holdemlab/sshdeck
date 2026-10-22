// Package ui is the Bubble Tea interface: a tab bar, a session tree on the
// left and terminal tabs on the right, each tab being a built-in SSH
// connection rendered through a terminal emulator.
package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/sshclient"

	"github.com/holdemlab/sshdeck/internal/i18n"
)

type focus int

const (
	focusSidebar focus = iota
	focusTerm
)

type sideMode int

const (
	sideBrowse sideMode = iota
	sideFilter
	sideForm
)

type overlay int

const (
	overlayNone overlay = iota
	overlayHelp
	overlayConfirmClose
	overlayConfirmQuit
	overlayTunnels
	overlayConfirmDelete
	overlayInstallKey
	overlayColors
)

// Options configure the UI.
type Options struct {
	ConfigPath   string // sessions.yaml
	SettingsPath string // config.yaml
	StatePath    string // state.yaml (folded folders); empty: not remembered
	Settings     config.Settings
	Notifier     *Notifier
	StartupError string // shown in the status bar on start
	Version      string // build version, shown in the tab bar ("v1.2.3", "dev+abc1234")
}

// Model is the root Bubble Tea model.
type Model struct {
	opts Options
	cfg  *config.Config

	// sidebar
	rows      []node
	cursor    int
	offset    int
	collapsed map[string]bool
	side      sideMode
	filter    textinput.Model
	form      form

	// tabs
	tabs      []*Tab
	active    int
	nextID    int
	scrolling bool

	// pending delete (overlayConfirmDelete)
	delNode node

	// quick tree edits
	inline inlineEdit
	clip   *clipboard // cut or copied, waiting for paste

	// passwords / passphrases remembered while sshdeck runs
	secrets *sshclient.Secrets

	colors colorDialog // C in the tree (overlayColors)
	// install-key dialog (overlayInstallKey)
	key keyDialog

	// help screen (overlayHelp)
	help  helpView
	mouse mouseState // double clicks, sidebar drag

	darkBG bool // terminal background, for the help theme
	// colors256: the terminal has 256 colors, not truecolor (themes then
	// pick the nearest colors themselves)
	colors256 bool

	// settings screen
	settingsOpen bool
	settings     settingsForm

	// tunnels panel
	tunCursor int
	tunAdding bool
	tunInput  textinput.Model
	tunErr    string
	tunAsk    bool // "discard this tunnel?" is showing

	focus       focus
	showSidebar bool
	prefixArmed bool
	overlay     overlay

	status   string
	statusOK bool

	width, height int
}

// New loads sessions.yaml and builds the initial model.
func New(opts Options) (Model, error) {
	i18n.Set(i18n.Resolve(opts.Settings.Language))
	useTheme(opts.Settings.Theme)
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = T("search…")
	tin := textinput.New()
	tin.Prompt = "> "
	tin.Placeholder = "L 8080:localhost:80  ·  D 1080  ·  R 9000:localhost:3000"
	tin.SetWidth(52)
	m := Model{
		secrets:     sshclient.NewSecrets(),
		darkBG:      true,
		opts:        opts,
		collapsed:   map[string]bool{},
		filter:      ti,
		tunInput:    tin,
		showSidebar: true,
		width:       80,
		height:      24,
	}
	if err := m.reload(); err != nil {
		return m, err
	}
	if opts.StartupError != "" {
		m.setStatus(false, "%s", opts.StartupError)
	}
	m.loadState()
	return m, nil
}

func (m *Model) reload() error {
	cfg, err := config.Load(m.opts.ConfigPath)
	if err != nil {
		return err
	}
	m.cfg = cfg
	m.refreshRows()
	m.refreshTabSessions(func(id string) string { return id })
	return nil
}

func (m *Model) setStatus(ok bool, format string, a ...any) {
	m.status, m.statusOK = T(format, a...), ok // format is translated
}

// Init implements tea.Model.
// Init asks the terminal for its background colour, so the help can pick a
// light or dark theme.
func (m Model) Init() tea.Cmd { return tea.RequestBackgroundColor }

// ---- layout ----

func (m Model) sidebarWidth() int {
	if !m.showSidebar {
		return 0
	}
	w := m.opts.Settings.SidebarWidth
	if w > m.width-20 {
		w = m.width - 20
	}
	if w < 16 {
		w = 16
	}
	return w
}

// termRect is where the active terminal is drawn.
func (m Model) termRect() uv.Rectangle {
	sw := m.sidebarWidth()
	return uv.Rect(sw, 1, max(m.width-sw, 1), max(m.height-2, 1))
}

func (m *Model) resizeTabs() {
	r := m.termRect()
	for _, t := range m.tabs {
		t.resize(r.Dx(), r.Dy())
	}
}

func (m Model) activeTab() *Tab {
	if m.active >= 0 && m.active < len(m.tabs) {
		return m.tabs[m.active]
	}
	return nil
}

// ---- update ----

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeTabs()
		m.clampCursor()
		if m.overlay == overlayHelp {
			m.renderHelp()
		}
		return m, nil

	case tea.ColorProfileMsg:
		m.colors256 = msg.Profile == colorprofile.ANSI256
		return m, nil
	case tea.BackgroundColorMsg:
		m.darkBG = msg.IsDark()
		termDark = m.darkBG
		if m.overlay == overlayHelp {
			m.renderHelp()
		}
		return m, nil

	case keyInstalledMsg:
		return m.keyInstalled(msg), nil

	case editorDoneMsg:
		return m.editorDone(msg), nil

	case tunnelTickMsg:
		if m.overlay == overlayTunnels {
			return m, tunnelTick() // keep the counters fresh
		}
		return m, nil

	case tabEventMsg:
		for _, t := range m.tabs {
			if t.id == msg.id {
				t.dirty.Store(false)
			}
		}
		return m, nil

	case tea.PasteMsg:
		if m.settingsOpen {
			return m.updateSettings(msg)
		}
		if m.overlay == overlayTunnels && m.tunAdding {
			var cmd tea.Cmd
			m.tunInput, cmd = m.tunInput.Update(msg)
			return m, cmd
		}
		if m.focus == focusTerm && m.overlay == overlayNone {
			if t := m.activeTab(); t != nil {
				t.paste(msg.Content)
			}
			return m, nil
		}
		return m.updateSidebar(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	if m.focus == focusSidebar {
		return m.updateSidebar(msg)
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayHelp:
		return m.helpKey(k)
	case overlayConfirmClose:
		m.overlay = overlayNone
		if k.String() == "y" {
			m.closeTab(m.active)
		}
		return m, nil
	case overlayConfirmQuit:
		m.overlay = overlayNone
		if k.String() == "y" {
			return m.quit()
		}
		return m, nil
	case overlayTunnels:
		return m.tunnelKey(k)
	case overlayInstallKey:
		return m.keyDialogKey(k)
	case overlayColors:
		return m.colorKey(k)
	case overlayConfirmDelete:
		m.overlay = overlayNone
		if k.String() == "y" {
			m.deleteNode(m.delNode)
		} else {
			m.setStatus(true, "delete cancelled")
		}
		return m, nil
	}

	if m.settingsOpen {
		return m.updateSettings(k)
	}
	if m.prefixArmed {
		m.prefixArmed = false
		return m.prefixCommand(k)
	}
	if k.String() == m.opts.Settings.Prefix {
		m.prefixArmed = true
		return m, nil
	}

	if m.focus == focusTerm {
		t := m.activeTab()
		if t == nil {
			m.focus = focusSidebar
			return m.updateSidebar(k)
		}
		if m.scrolling {
			m.scrollKey(t, k)
			return m, nil
		}
		if t.handleKey(k) {
			t.print("\n")
			m.connectTab(t)
		}
		return m, nil
	}
	return m.updateSidebar(k)
}

func (m Model) prefixCommand(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s == m.opts.Settings.Prefix {
		if t := m.activeTab(); t != nil && m.focus == focusTerm {
			t.sendKey(k)
		}
		return m, nil
	}
	switch s {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		i, _ := strconv.Atoi(s)
		if i-1 < len(m.tabs) {
			m.switchTab(i - 1)
		}
	case "n", "right", "tab":
		if len(m.tabs) > 0 {
			m.switchTab((m.active + 1) % len(m.tabs))
		}
	case "p", "left", "shift+tab":
		if len(m.tabs) > 0 {
			m.switchTab((m.active - 1 + len(m.tabs)) % len(m.tabs))
		}
	case "s":
		if m.focus == focusSidebar && len(m.tabs) > 0 {
			m.focus = focusTerm
		} else {
			m.focus = focusSidebar
			if !m.showSidebar {
				m.showSidebar = true
				m.resizeTabs()
			}
		}
	case "b":
		if len(m.tabs) > 0 {
			m.showSidebar = !m.showSidebar
			if !m.showSidebar {
				m.focus = focusTerm
			}
			m.resizeTabs()
		}
	case "w", "x":
		if t := m.activeTab(); t != nil {
			if t.live() && m.opts.Settings.ConfirmClose {
				m.overlay = overlayConfirmClose
			} else {
				m.closeTab(m.active)
			}
		}
	case "r":
		if t := m.activeTab(); t != nil {
			if t.live() {
				m.setStatus(false, "%s is still connected", t.title())
			} else {
				t.print("\n")
				m.connectTab(t)
				m.focus = focusTerm
			}
		}
	case "d":
		if t := m.activeTab(); t != nil && t.local {
			m.openLocalTab()
		} else if t != nil {
			m.openTab(t.session, t.tunnelOnly)
		}
	case "l":
		m.openLocalTab()
	case "[":
		if t := m.activeTab(); t != nil {
			m.scrolling = true
			m.focus = focusTerm
		}
	case "t":
		if m.activeTab() == nil {
			m.setStatus(false, "open a session first (t in the sidebar opens tunnels only)")
		} else if m.activeTab().local {
			m.setStatus(false, "tunnels need an SSH session; the local shell has none")
		} else {
			m.overlay, m.tunCursor, m.tunAdding, m.tunErr = overlayTunnels, 0, false, ""
			return m, tunnelTick()
		}
	case ",":
		m.openSettings()
	case "e":
		if t := m.activeTab(); t != nil && t.local {
			m.openSettings()
			m.settings.setFocus(sLocal)
		} else {
			m.editTabSession()
		}
	case "k":
		if t := m.activeTab(); t != nil && t.local {
			m.setStatus(false, "the local shell needs no key; open an SSH session to install one")
			return m, nil
		}
		return m, m.openInstallKey()
	case "?", "h":
		m.openHelp()
	case "q":
		return m.requestQuit()
	}
	return m, nil
}

func (m *Model) openSettings() {
	m.settings = newSettingsForm(m.opts.Settings)
	m.settingsOpen = true
	m.overlay = overlayNone
}

func (m Model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	f, cmd, action := m.settings.update(msg)
	m.settings = f
	switch action {
	case saCancel:
		m.settingsOpen = false
	case saEditor:
		m.settingsOpen = false
		return m, openEditor(m.opts.SettingsPath, editSettings)
	case saSave:
		s, err := f.result(m.opts.Settings)
		if err == nil {
			err = config.SaveSettings(m.opts.SettingsPath, s)
		}
		if err != nil {
			m.settings.err = err.Error()
			return m, nil
		}
		m.applySettings(s)
		m.settingsOpen = false
		m.setStatus(true, "settings saved to %s", m.opts.SettingsPath)
	}
	return m, cmd
}

// applySettings switches to new settings without a restart. Layout and
// scrollback change at once; connection options apply to new connections.
// connectTab starts (or restarts) a tab's connection.
func (m *Model) connectTab(t *Tab) {
	var sec *sshclient.Secrets
	if m.opts.Settings.RememberPasswords {
		sec = m.secrets
	}
	t.connect(m.cfg, m.opts.Settings, sec)
}

func (m *Model) applySettings(s config.Settings) {
	if !s.RememberPasswords {
		m.secrets.Clear() // turning it off forgets everything at once
	}
	m.opts.Settings = s
	i18n.Set(i18n.Resolve(s.Language))
	useTheme(s.Theme)
	m.filter.Placeholder = T("search…")
	m.help.key = "" // re-render the help in the new language
	for _, t := range m.tabs {
		t.emu.SetScrollbackSize(s.Scrollback)
		themeEmulator(t.emu, curTheme)
		if t.local {
			t.session = localSession(s) // new colors; the command applies to new tabs
		}
	}
	m.resizeTabs()
	m.refreshRows() // the local entry may have been turned on or off
}

func (m Model) editorDone(msg editorDoneMsg) Model {
	if msg.err != nil {
		m.setStatus(false, "editor: %v", msg.err)
		return m
	}
	switch msg.target {
	case editSettings:
		s, err := config.LoadSettings(m.opts.SettingsPath, false)
		if err != nil {
			m.setStatus(false, "%v — previous settings kept", err)
			return m
		}
		m.applySettings(s)
		m.setStatus(true, "settings reloaded from %s", m.opts.SettingsPath)
	case editSessions:
		if err := m.reload(); err != nil {
			m.setStatus(false, "%v — fix it with E", err)
			return m
		}
		m.setStatus(true, "sessions reloaded from %s", m.opts.ConfigPath)
	}
	return m
}

func (m *Model) switchTab(i int) {
	m.active = i
	m.focus = focusTerm
	m.scrolling = false
}

// openTab creates a tab for s and starts connecting. A tunnelOnly tab opens
// no shell and only carries the session's forwards.
func (m *Model) openTab(s config.Resolved, tunnelOnly bool) {
	r := m.termRect()
	m.nextID++
	t := newTab(m.nextID, s, r.Dx(), r.Dy(), m.opts.Settings.Scrollback, m.opts.Notifier, tunnelOnly)
	m.tabs = append(m.tabs, t)
	m.active = len(m.tabs) - 1
	m.focus = focusTerm
	m.scrolling = false
	m.connectTab(t)
}

func (m *Model) closeTab(i int) {
	if i < 0 || i >= len(m.tabs) {
		return
	}
	t := m.tabs[i]
	t.close()
	m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
	if m.active >= len(m.tabs) {
		m.active = len(m.tabs) - 1
	}
	m.scrolling = false
	if len(m.tabs) == 0 {
		m.active = 0
		m.focus = focusSidebar
		m.showSidebar = true
		m.resizeTabs()
	}
	m.setStatus(true, "closed %s", t.title())
}

func (m Model) liveTabs() int {
	n := 0
	for _, t := range m.tabs {
		if t.live() {
			n++
		}
	}
	return n
}

func (m Model) requestQuit() (tea.Model, tea.Cmd) {
	if m.liveTabs() > 0 {
		m.overlay = overlayConfirmQuit
		return m, nil
	}
	return m.quit()
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	for _, t := range m.tabs {
		t.close()
	}
	return m, tea.Quit
}

func (m *Model) scrollKey(t *Tab, k tea.KeyPressMsg) {
	page := max(m.termRect().Dy()-1, 1)
	maxScroll := t.emu.ScrollbackLen()
	switch k.String() {
	case "up", "k":
		t.scroll++
	case "down", "j":
		t.scroll--
	case "pgup", "ctrl+b", "ctrl+u":
		t.scroll += page
	case "pgdown", "ctrl+f", "ctrl+d", "space":
		t.scroll -= page
	case "g", "home":
		t.scroll = maxScroll
	case "G", "end":
		t.scroll = 0
	case "esc", "q", "enter":
		t.scroll = 0
		m.scrolling = false
	}
	t.scroll = min(max(t.scroll, 0), maxScroll)
}

// ---- sidebar ----

func (m *Model) refreshRows() {
	if m.side == sideFilter || m.filter.Value() != "" {
		m.rows = m.withLocal(filterNodes(allSessions(m.cfg), m.filter.Value()), m.filter.Value())
	} else {
		m.rows = m.withLocal(buildTree(m.cfg, m.collapsed), "")
	}
	m.clampCursor()
}

// treeHeight is the number of tree rows that fit in the sidebar.
func (m Model) treeHeight() int {
	return max(m.height-2 /*bars*/ -2 /*title+search*/ -detailLines, 1)
}

const detailLines = 5

func (m *Model) clampCursor() {
	m.cursor = min(m.cursor, len(m.rows)-1)
	m.cursor = max(m.cursor, 0)
	h := m.treeHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
}

func (m Model) current() (node, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return node{}, false
	}
	return m.rows[m.cursor], true
}

func (m Model) updateSidebar(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.side {
	case sideForm:
		return m.updateForm(msg)
	case sideFilter:
		return m.updateFilter(msg)
	}
	if m.inline.mode != inlineNone {
		return m.updateInline(msg)
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if m.clip == nil || k.String() != "esc" {
		m.status = ""
	}
	if n, ok := m.current(); ok && n.kind == localNode {
		if lm, cmd, handled := m.localKey(k); handled {
			return lm, cmd
		}
	}
	switch k.String() {
	case "q", "ctrl+c":
		return m.requestQuit()
	case "tab", "esc":
		if k.String() == "esc" && m.clip != nil {
			if m.clip.copy {
				m.setStatus(true, "copy done")
			} else {
				m.setStatus(true, "move cancelled")
			}
			m.clip = nil
		} else if k.String() == "esc" && m.filter.Value() != "" {
			m.filter.SetValue("")
			m.refreshRows()
		} else if len(m.tabs) > 0 {
			m.focus = focusTerm
		}
	case "up", "k", "ctrl+p":
		m.cursor--
		m.clampCursor()
	case "down", "j", "ctrl+n":
		m.cursor++
		m.clampCursor()
	case "home", "g":
		m.cursor = 0
		m.clampCursor()
	case "end", "G":
		m.cursor = len(m.rows) - 1
		m.clampCursor()
	case "left", "h":
		m.collapse()
	case "right", "l":
		if n, ok := m.current(); ok && n.kind == folderNode && m.collapsed[n.key] {
			m.setCollapsed(n.key, false)
		}
	case "H", "-":
		m.collapseAll()
	case "L", "*":
		m.expandAll()
	case "enter":
		m.activate(false)
	case "t":
		m.activate(true)
	case "/":
		m.side = sideFilter
		m.cursor = 0
		m.refreshRows()
		return m, m.filter.Focus()
	case "n":
		var path []string
		if n, ok := m.current(); ok {
			path = n.path
		}
		m.form = newForm(T("New session"), path, nil, nil)
		m.side = sideForm
	case "N", "+":
		return m, m.startNewFolder()
	case "R", "f2":
		return m, m.startRename()
	case "x":
		m.cut()
	case "p":
		m.paste(false)
	case "P":
		m.paste(true)
	case "e":
		m.editNode()
	case "c":
		return m, m.duplicate()
	case "y":
		m.yank()
	case "d", "delete":
		if n, ok := m.current(); ok {
			m.delNode, m.overlay = n, overlayConfirmDelete
		} else {
			m.setStatus(false, "nothing selected")
		}
	case "r":
		if err := m.reload(); err != nil {
			m.setStatus(false, "%v", err)
		} else {
			m.setStatus(true, "reloaded %s", m.opts.ConfigPath)
		}
	case "C":
		m.openColorDialog()
		return m, nil
	case "E":
		return m, openEditor(m.opts.ConfigPath, editSessions)
	case ",":
		m.openSettings()
	case "?":
		m.openHelp()
	}
	return m, nil
}

func (m *Model) collapse() {
	n, ok := m.current()
	if !ok || m.filter.Value() != "" {
		return
	}
	if n.kind == folderNode && !m.collapsed[n.key] {
		m.setCollapsed(n.key, true)
		return
	}
	for i := m.cursor - 1; i >= 0; i-- {
		if m.rows[i].kind == folderNode && m.rows[i].depth < n.depth {
			m.cursor = i
			m.clampCursor()
			return
		}
	}
}

// activate opens a session in a new tab or toggles a folder.
func (m *Model) activate(tunnelOnly bool) {
	n, ok := m.current()
	if !ok {
		return
	}
	if n.kind == folderNode {
		m.setCollapsed(n.key, !m.collapsed[n.key])
		return
	}
	r, err := m.cfg.Resolve(n.path, n.session)
	if err != nil {
		m.setStatus(false, "%v", err)
		return
	}
	m.openTab(r, tunnelOnly)
}

func (m Model) updateFilter(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			m.filter.SetValue("")
			m.filter.Blur()
			m.side = sideBrowse
			m.refreshRows()
			return m, nil
		case "enter":
			m.filter.Blur()
			m.side = sideBrowse
			m.activate(false)
			if m.focus == focusTerm {
				// clear the search once something was opened
				m.filter.SetValue("")
				m.refreshRows()
			}
			return m, nil
		case "up", "ctrl+p":
			m.cursor--
			m.clampCursor()
			return m, nil
		case "down", "ctrl+n", "tab":
			m.cursor++
			m.clampCursor()
			return m, nil
		case "ctrl+c":
			return m.requestQuit()
		}
	}
	var cmd tea.Cmd
	before := m.filter.Value()
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != before {
		m.cursor = 0
		m.refreshRows()
	}
	return m, cmd
}

func (m Model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	f, cmd, submit, cancel := m.form.update(msg)
	m.form = f
	if cancel {
		m.side = sideBrowse
		return m, nil
	}
	if !submit {
		return m, cmd
	}
	if m.form.folderMode {
		return m.saveFolderForm()
	}
	path, s, err := m.form.result()
	if err == nil {
		err = m.saveSession(path, s, m.form.editing)
	}
	if err != nil {
		m.form.err = err.Error()
		return m, nil
	}
	m.side = sideBrowse
	m.filter.SetValue("")
	m.refreshRows()
	m.selectSession(path, s.Name)
	m.setStatus(true, "saved %s", s.Name)
	return m, nil
}

// saveSession applies the change to a fresh copy of the file, so a failed
// save never leaves the in-memory tree half-modified.
func (m *Model) saveSession(path []string, s *config.Session, editing *sessionRef) error {
	cfg, err := config.Load(m.opts.ConfigPath)
	if err != nil {
		return err
	}
	oldName := ""
	if editing != nil {
		if strings.Join(editing.path, "/") == strings.Join(path, "/") {
			oldName = editing.name
		} else if err := cfg.Delete(editing.path, editing.name); err != nil {
			return err
		}
	}
	if err := cfg.Put(path, oldName, s); err != nil {
		return err
	}
	rename := func(id string) string { return id }
	if editing != nil {
		oldID := strings.Join(append(append([]string{}, editing.path...), editing.name), "/")
		newID := strings.Join(append(append([]string{}, path...), s.Name), "/")
		if oldID != newID {
			cfg.RenameRefs(oldID, newID, false)
			rename = func(id string) string {
				if id == oldID {
					return newID
				}
				return id
			}
		}
	}
	if err := config.Save(m.opts.ConfigPath, cfg); err != nil {
		return err
	}
	m.cfg = cfg
	m.refreshTabSessions(rename)
	return nil
}

// refreshTabSessions gives open tabs the current settings of their session
// (used on the next reconnect). rename maps old session ids to new ones.
func (m *Model) refreshTabSessions(rename func(string) string) {
	for _, t := range m.tabs {
		if t.local {
			continue
		}
		if r, ok := m.cfg.Find(rename(t.session.ID())); ok {
			t.session = r
		}
	}
}

// editNode opens the form for the selected session or folder.
func (m *Model) editNode() {
	n, ok := m.current()
	if !ok {
		m.setStatus(false, "nothing selected")
		return
	}
	if n.kind == folderNode {
		f := m.cfg.FolderAt(n.path)
		if f == nil {
			return
		}
		m.form = newFolderForm(T("Edit folder %s", n.name), n.path, f)
	} else {
		cp := *n.session
		m.form = newForm(T("Edit %s", n.name), n.path, &cp, &sessionRef{path: n.path, name: n.name})
	}
	m.side = sideForm
}

// editTabSession edits the session of the active tab (prefix + e).
func (m *Model) editTabSession() {
	t := m.activeTab()
	if t == nil {
		m.setStatus(false, "no tab open")
		return
	}
	s := m.cfg.Session(t.session.Path, t.session.Name)
	if s == nil {
		m.setStatus(false, "%s is no longer in %s", t.session.ID(), m.opts.ConfigPath)
		return
	}
	cp := *s
	m.form = newForm(T("Edit %s", s.Name), t.session.Path, &cp, &sessionRef{path: t.session.Path, name: s.Name})
	m.side, m.focus = sideForm, focusSidebar
	if !m.showSidebar {
		m.showSidebar = true
		m.resizeTabs()
	}
}

func (m Model) saveFolderForm() (tea.Model, tea.Cmd) {
	newPath, d, err := m.form.folderResult()
	if err != nil {
		m.form.err = err.Error()
		return m, nil
	}
	cfg, err := config.Load(m.opts.ConfigPath)
	if err != nil {
		m.form.err = err.Error()
		return m, nil
	}
	rename := func(id string) string { return id }
	if e := m.form.editing; e != nil {
		err = cfg.UpdateFolder(e.path, newPath, d)
		oldID, newID := strings.Join(e.path, "/"), strings.Join(newPath, "/")
		rename = func(id string) string {
			if strings.HasPrefix(id, oldID+"/") {
				return newID + strings.TrimPrefix(id, oldID)
			}
			return id
		}
	} else {
		err = cfg.AddFolder(newPath, d)
	}
	if err == nil {
		err = cfg.Validate()
	}
	if err == nil {
		err = config.Save(m.opts.ConfigPath, cfg)
	}
	if err != nil {
		m.form.err = err.Error()
		return m, nil
	}
	m.cfg = cfg
	if e := m.form.editing; e != nil {
		m.moveCollapsed(strings.Join(e.path, "/"), strings.Join(newPath, "/"), false)
	}
	m.refreshTabSessions(rename)
	m.side = sideBrowse
	m.filter.SetValue("")
	m.refreshRows()
	key := "f:" + strings.Join(newPath, "/")
	for i, n := range m.rows {
		if n.key == key {
			m.cursor = i
			m.clampCursor()
		}
	}
	m.setStatus(true, "saved folder %s", strings.Join(newPath, "/"))
	return m, nil
}

// deleteNode removes a session or a whole folder from sessions.yaml.
func (m *Model) deleteNode(n node) {
	cfg, err := config.Load(m.opts.ConfigPath)
	if err == nil {
		if n.kind == folderNode {
			err = cfg.DeleteFolder(n.path)
		} else {
			err = cfg.Delete(n.path, n.name)
		}
	}
	if err == nil {
		err = config.Save(m.opts.ConfigPath, cfg)
	}
	if err != nil {
		m.setStatus(false, "delete failed: %v", err)
		return
	}
	m.cfg = cfg
	m.refreshRows()
	m.saveState() // drops folds of deleted folders
	m.setStatus(true, "deleted %s", n.id())
}

func (m Model) renderConfirmDelete() string {
	n := m.delNode
	var b strings.Builder
	if n.kind == folderNode {
		b.WriteString(warnStyle.Render(T("Delete folder %s?", n.id())) + "\n\n")
		b.WriteString(T("Everything inside goes too: %d session(s).", n.count) + "\n")
		if u := m.cfg.JumpUsers(n.id(), true); u > 0 {
			b.WriteString(errStyle.Render(T("%d other place(s) jump through sessions in it.", u)) + "\n")
		}
	} else {
		b.WriteString(warnStyle.Render(T("Delete session %s?", n.id())) + "\n\n")
		if n.session != nil {
			b.WriteString(mutedStyle.Render(n.session.Host) + "\n")
		}
		if u := m.cfg.JumpUsers(n.id(), false); u > 0 {
			b.WriteString(errStyle.Render(T("%d other place(s) use it as a jump host.", u)) + "\n")
		}
	}
	if m.liveTabsUnder(n) > 0 {
		b.WriteString(mutedStyle.Render(T("Open tabs stay connected.")) + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render(T("y delete · any other key cancels")))
	return b.String()
}

func (m Model) liveTabsUnder(n node) int {
	c := 0
	for _, t := range m.tabs {
		id := t.session.ID()
		if t.live() && (id == n.id() || (n.kind == folderNode && strings.HasPrefix(id, n.id()+"/"))) {
			c++
		}
	}
	return c
}

func (m *Model) selectSession(path []string, name string) {
	want := strings.Join(path, "/")
	for i, n := range m.rows {
		if n.session != nil && n.name == name && strings.Join(n.path, "/") == want {
			m.cursor = i
			m.clampCursor()
			return
		}
	}
}

// ---- view ----

// View implements tea.Model.
func (m Model) View() tea.View {
	// the settings screen previews the theme being chosen
	if m.settingsOpen {
		useTheme(m.settings.previewTheme())
	} else {
		useTheme(m.opts.Settings.Theme)
	}
	if curTheme != nil {
		m.styleInputs()
	}
	canvas := lipgloss.NewCanvas(m.width, m.height)
	draw := func(s string, r uv.Rectangle) { uv.NewStyledString(s).Draw(canvas, r) }

	draw(m.renderTabBar(), uv.Rect(0, 0, m.width, 1))
	tr := m.termRect()
	if sw := m.sidebarWidth(); sw > 0 {
		draw(m.renderSidebar(sw, m.height-2), uv.Rect(0, 1, sw, m.height-2))
	}

	var cursor *tea.Cursor
	t := m.activeTab()
	switch {
	case m.settingsOpen:
		draw(lipgloss.NewStyle().Padding(1, 2).Render(m.settings.view(m.opts.SettingsPath)), tr)
	case m.side == sideForm:
		draw(lipgloss.NewStyle().Padding(1, 2).Render(m.form.view()), tr)
	case t != nil:
		m.drawTab(canvas, t, tr)
		if m.focus == focusTerm && !m.scrolling && m.overlay == overlayNone && !m.settingsOpen && !t.emu.CursorHidden() {
			p := t.emu.CursorPosition()
			cursor = tea.NewCursor(tr.Min.X+p.X, tr.Min.Y+p.Y)
		}
	default:
		draw(m.renderWelcome(), tr)
	}
	draw(m.renderStatusBar(), uv.Rect(0, m.height-1, m.width, 1))

	switch m.overlay {
	case overlayHelp:
		m.drawCentered(canvas, m.renderHelpBox())
	case overlayConfirmClose:
		if t != nil {
			m.drawCentered(canvas, overlayStyle.Render(warnStyle.Render(T("Close %s?", t.title()))+"\n\n"+T("The connection is still open.")+"\n\n"+mutedStyle.Render(T("y close · any other key cancels"))))
		}
	case overlayConfirmDelete:
		m.drawCentered(canvas, overlayStyle.Render(m.renderConfirmDelete()))
	case overlayInstallKey:
		m.drawCentered(canvas, overlayStyle.Render(m.renderKeyDialog()))
	case overlayColors:
		m.drawCentered(canvas, overlayStyle.Render(m.renderColorDialog()))
	case overlayTunnels:
		if t != nil {
			m.drawCentered(canvas, overlayStyle.Render(m.renderTunnels(t)))
		}
	case overlayConfirmQuit:
		m.drawCentered(canvas, overlayStyle.Render(warnStyle.Render(T("Quit sshdeck?"))+"\n\n"+T("%d connection(s) are still open.", m.liveTabs())+"\n\n"+mutedStyle.Render(T("y quit · any other key cancels"))))
	}

	curTheme.paint(canvas, m.colors256)
	if cursor != nil {
		cursor.Color = curTheme.cursorColor()
	}
	v := tea.NewView(canvas.Render())
	v.AltScreen = true
	if m.opts.Settings.Mouse {
		v.MouseMode = tea.MouseModeCellMotion // clicks, wheel, drags
	}
	v.Cursor = cursor
	v.WindowTitle = "sshdeck"
	if t != nil {
		v.WindowTitle = "sshdeck — " + t.title()
	}
	return v
}

func (m Model) drawCentered(canvas *lipgloss.Canvas, s string) {
	w, h := lipgloss.Width(s), lipgloss.Height(s)
	x, y := max((m.width-w)/2, 0), max((m.height-h)/2, 0)
	uv.NewStyledString(s).Draw(canvas, uv.Rect(x, y, w, h))
}

// drawTab paints the tab's screen, or a scrolled-back view of it.
func (m Model) drawTab(canvas *lipgloss.Canvas, t *Tab, r uv.Rectangle) {
	if !m.scrolling || t.scroll == 0 {
		t.emu.Draw(canvas, r)
		if m.scrolling {
			m.drawScrollBadge(canvas, t, r)
		}
		return
	}
	sb := t.emu.ScrollbackLen()
	w, h := t.emu.Width(), t.emu.Height()
	for row := 0; row < r.Dy() && row < h; row++ {
		line := sb - t.scroll + row // index into scrollback ++ screen
		for x := 0; x < w && x < r.Dx(); x++ {
			var c *uv.Cell
			if line < sb {
				c = t.emu.ScrollbackCellAt(x, line)
			} else {
				c = t.emu.CellAt(x, line-sb)
			}
			if c != nil {
				canvas.SetCell(r.Min.X+x, r.Min.Y+row, c.Clone())
			}
		}
	}
	m.drawScrollBadge(canvas, t, r)
}

func (m Model) drawScrollBadge(canvas *lipgloss.Canvas, t *Tab, r uv.Rectangle) {
	badge := prefixStyle.Render(T("scroll %d/%d · esc", t.scroll, t.emu.ScrollbackLen()))
	w := lipgloss.Width(badge)
	uv.NewStyledString(badge).Draw(canvas, uv.Rect(r.Max.X-w, r.Min.Y, w, 1))
}

func (m Model) renderTabBar() string {
	parts := m.tabBarParts()
	bar := strings.Join(parts, barStyle.Render(" "))
	if lipgloss.Width(bar) > m.width {
		bar = ansi.Truncate(bar, m.width-1, "…")
	}
	// the build version sits at the right end while there is room for it
	if v := m.opts.Version; v != "" {
		ver := barStyle.Foreground(muted).Render(v + " ")
		if gap := m.width - lipgloss.Width(bar) - lipgloss.Width(ver); gap >= 2 {
			bar += barStyle.Render(strings.Repeat(" ", gap)) + ver
		}
	}
	return barStyle.Width(m.width).Render(bar)
}

// tabSpans are the columns [start, end) of each tab in the tab bar.
func (m Model) tabSpans() [][2]int {
	parts := m.tabBarParts()
	spans := make([][2]int, 0, len(m.tabs))
	x := lipgloss.Width(parts[0]) + 1
	for _, p := range parts[1:] {
		w := lipgloss.Width(p)
		spans = append(spans, [2]int{x, x + w})
		x += w + 1
	}
	return spans
}

// tabBarParts are the brand and one label per tab.
func (m Model) tabBarParts() []string {
	parts := []string{brandStyle.Render("sshdeck")}
	for i, t := range m.tabs {
		state, _ := t.getState()
		dot := okStyle.Render("●")
		switch state {
		case stateConnecting:
			dot = warnStyle.Render("◌")
		case stateClosed:
			dot = errStyle.Render("✗")
		}
		label := fmt.Sprintf("%d %s", i+1, t.title())
		if n := t.listening(); n > 0 && !t.tunnelOnly {
			label += fmt.Sprintf(" ⇄%d", n)
		}
		parts = append(parts, tabLabel(t, i == m.active, label, dot))
	}
	return parts
}

func (m Model) renderStatusBar() string {
	var left string
	switch {
	case m.prefixArmed:
		left = prefixStyle.Render(m.opts.Settings.Prefix) + " " + T("1-9 tab · n/p · s sidebar · l local shell · t tunnels · e edit session · k install key · , settings · b hide · w close · r reconnect · d dup · [ scroll · ? help · q quit")
	case m.focus == focusTerm && m.activeTab() != nil:
		t := m.activeTab()
		_, reason := t.getState()
		info := t.routeString()
		if n := t.listening(); n > 0 {
			info += mutedStyle.Render("  " + T("⇄ %d tunnel(s)", n))
		}
		if ev := t.takeLastEvent(); ev != "" {
			info += "  " + errStyle.Render(ev)
		}
		if reason != "" && reason != "closed" {
			info = errStyle.Render(reason)
		}
		left = mutedStyle.Render(m.opts.Settings.Prefix+" ? "+T("help")) + "  " + info +
			mutedStyle.Render(fmt.Sprintf("  %d×%d", t.emu.Width(), t.emu.Height()))
	default:
		left = mutedStyle.Render(T("⏎ open · n new · N folder · R rename · x/p move · y/p copy · c dup · e edit · C colors · d delete · / search · t tunnels · E file · , settings · %s ? help · q quit", m.opts.Settings.Prefix))
	}
	if m.status != "" && !m.prefixArmed {
		st := okStyle.Render("● " + m.status)
		if !m.statusOK {
			st = errStyle.Render("✗ " + m.status)
		}
		left = st + "  " + left
	}
	return barStyle.Width(m.width).Render(ansi.Truncate(left, m.width, "…"))
}

func (m Model) renderSidebar(w, h int) string {
	inner := w - 3 // border + padding
	var b strings.Builder
	title := titleStyle.Render(T("Sessions"))
	if m.focus != focusSidebar {
		title = mutedStyle.Bold(true).Render(T("Sessions"))
	}
	b.WriteString(title + mutedStyle.Render(fmt.Sprintf(" %d", totalSessions(m.cfg))) + "\n")
	if m.side == sideFilter || m.filter.Value() != "" {
		b.WriteString(ansi.Truncate(m.filter.View(), inner, "…"))
	} else {
		b.WriteString(mutedStyle.Render(T("/ search")))
	}
	b.WriteString("\n")

	th := m.treeHeight()
	end := min(m.offset+th, len(m.rows))
	lines := 0
	if len(m.rows) == 0 {
		if m.filter.Value() != "" {
			b.WriteString(mutedStyle.Render(T("no matches")) + "\n")
		} else {
			b.WriteString(mutedStyle.Render(T("no sessions — press n")) + "\n")
		}
		lines++
	}
	flat := m.filter.Value() != ""
	if m.inline.mode == inlineNewFolder && len(m.rows) == 0 {
		b.WriteString(m.inlineRow(0, inner) + "\n")
		lines++
	}
	for i := m.offset; i < end && lines < th; i++ {
		n := m.rows[i]
		if m.inline.mode == inlineRename && i == m.cursor {
			b.WriteString(m.inlineRow(n.depth, inner) + "\n")
			lines++
			continue
		}
		b.WriteString(m.renderRow(n, i == m.cursor && m.focus == focusSidebar, i == m.cursor, flat, inner) + "\n")
		lines++
		if m.inline.mode == inlineNewFolder && i == m.cursor && lines < th {
			depth := n.depth
			if n.kind == folderNode {
				depth++
			}
			b.WriteString(m.inlineRow(depth, inner) + "\n")
			lines++
		}
	}
	for ; lines < th; lines++ {
		b.WriteString("\n")
	}
	b.WriteString(m.renderDetails(inner))
	return sidebarStyle.Width(w).Height(h).MaxHeight(h).Render(b.String())
}

func (m Model) renderRow(n node, selected, current, flat bool, width int) string {
	indent := strings.Repeat("  ", n.depth)
	st, marked, filled := m.nodeMark(n)
	// pre + name + post; the name carries the color mark
	var pre, name, post string
	switch n.kind {
	case folderNode:
		arrow := "▾ "
		if m.collapsed[n.key] {
			arrow = "▸ "
		}
		pre = indent + arrow
		name = folderStyle.Inherit(st).Render(n.name)
		if marked {
			name = chip(st.Bold(true), n.name, filled)
		}
		post = mutedStyle.Render(" " + strconv.Itoa(n.count))
	case localNode:
		mark := "  "
		if m.hasOpenLocalTab() {
			mark = okStyle.Render("● ")
		}
		pre = mark
		name = n.name
		if marked {
			name = chip(st, n.name, filled)
		}
		post = mutedStyle.Render(" " + T("this computer"))
	case sessionNode:
		mark := "  "
		if m.hasOpenTab(n.id()) {
			mark = okStyle.Render("● ")
		}
		pre = indent + mark
		if flat && len(n.path) > 0 {
			pre += mutedStyle.Render(strings.Join(n.path, "/") + "/")
		}
		name = n.name
		if marked {
			name = chip(st, n.name, filled)
		}
	}
	if m.clip != nil && m.clip.key == n.key {
		if m.clip.copy {
			post += warnStyle.Render(" ⧉")
		} else {
			post += warnStyle.Render(" ✂")
		}
	}
	if selected || current {
		row := lipgloss.NewStyle().Underline(true)
		if selected {
			row = selectedStyle
		}
		if !marked {
			name = row.Render(ansi.Strip(name))
		}
		pre, post = row.Render(ansi.Strip(pre)), ansi.Strip(post)
		text := ansi.Truncate(pre+name+row.Render(post), width, "…")
		pad := max(width-lipgloss.Width(text), 0)
		return text + row.Render(strings.Repeat(" ", pad))
	}
	return ansi.Truncate(pre+name+post, width, "…")
}

func (m Model) hasOpenTab(id string) bool {
	for _, t := range m.tabs {
		if t.session.ID() == id && t.live() {
			return true
		}
	}
	return false
}

// renderDetails shows the selected session at the bottom of the sidebar.
func (m Model) renderDetails(width int) string {
	lines := make([]string, 0, detailLines)
	lines = append(lines, mutedStyle.Render(strings.Repeat("─", width)))
	n, ok := m.current()
	switch {
	case ok && n.kind == localNode:
		lines = append(lines, m.localDetails()...)
	case ok && n.session != nil:
		r, err := m.cfg.Resolve(n.path, n.session)
		if err != nil {
			lines = append(lines, errStyle.Render(err.Error()))
			break
		}
		user := r.User
		if user == "" {
			user = "$USER"
		}
		port := ""
		if r.Port != 0 && r.Port != 22 {
			port = ":" + strconv.Itoa(r.Port)
		}
		lines = append(lines, user+"@"+r.Host+port)
		if r.Jump != "" {
			lines = append(lines, mutedStyle.Render(T("via")+" ")+r.Jump)
		}
		if r.Command != "" {
			lines = append(lines, mutedStyle.Render(T("runs")+" ")+r.Command)
		}
		if len(r.Forwards) > 0 {
			lines = append(lines, mutedStyle.Render("⇄ ")+strings.Join(r.Forwards, ", "))
		}
		if r.Note != "" {
			lines = append(lines, mutedStyle.Render(r.Note))
		}
	case ok:
		d := m.folderDefaults(n.path)
		if d.User != "" {
			lines = append(lines, mutedStyle.Render(T("user")+" ")+d.User)
		}
		if d.Jump != "" {
			lines = append(lines, mutedStyle.Render(T("via")+" ")+d.Jump)
		}
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	for len(lines) < detailLines {
		lines = append(lines, "")
	}
	return strings.Join(lines[:detailLines], "\n")
}

func (m Model) folderDefaults(path []string) config.Defaults {
	r, err := m.cfg.Resolve(path, &config.Session{})
	if err != nil {
		return config.Defaults{}
	}
	return r.Defaults
}

func (m Model) renderWelcome() string {
	p := m.opts.Settings.Prefix
	return lipgloss.NewStyle().Padding(1, 3).Render(
		titleStyle.Render("sshdeck") + "\n\n" +
			T("Pick a session on the left and press %s — it opens in a tab.", titleStyle.Render("enter")) + "\n" +
			T("New session: %s.  Help: %s.", titleStyle.Render("n"), titleStyle.Render(p+" ?")) + "\n\n" +
			mutedStyle.Render(T("Inside a tab every key goes to the server, except %s:", p)+"\n"+
				T("%[1]s 1-9 switch tab · %[1]s s sidebar · %[1]s w close · %[1]s [ scroll back", p)) + "\n\n" +
			mutedStyle.Render(T("Sessions: %s", m.opts.ConfigPath)))
}
