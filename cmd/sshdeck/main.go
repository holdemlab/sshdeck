// Command sshdeck is a terminal SSH session manager with built-in tabs.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/holdemlab/sshdeck/internal/buildinfo"
	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/i18n"
	"github.com/holdemlab/sshdeck/internal/ui"
)

func main() {
	defSessions, err := config.DefaultPath()
	if err != nil {
		fail(err)
	}
	defSettings, err := config.SettingsPath()
	if err != nil {
		fail(err)
	}
	sessionsPath := flag.String("sessions", defSessions, "path to sessions.yaml")
	settingsPath := flag.String("config", defSettings, "path to config.yaml (settings)")
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "print version, commit and build date, then exit")
	flag.BoolVar(&showVersion, "v", false, "same as -version")
	flag.Parse()

	build := buildinfo.Get()
	if showVersion {
		fmt.Print(build.Long())
		return
	}
	// A broken config.yaml should not lock you out of the settings screen:
	// start with defaults and show the error instead.
	startupErr := ""
	settings, err := config.LoadSettings(*settingsPath, *settingsPath == defSettings)
	if err != nil {
		settings = config.DefaultSettings()
		i18n.Set(i18n.Resolve(settings.Language))
		startupErr = i18n.T("%v — using defaults, fix it with ctrl+a ,", err)
	}

	statePath, err := config.StatePath()
	if err != nil {
		statePath = "" // folds are then not remembered
	}

	notifier := &ui.Notifier{}
	m, err := ui.New(ui.Options{
		ConfigPath:   *sessionsPath,
		SettingsPath: *settingsPath,
		StatePath:    statePath,
		Settings:     settings,
		Notifier:     notifier,
		StartupError: startupErr,
		Version:      build.Short(),
	})
	if err != nil {
		fail(err)
	}
	p := tea.NewProgram(m)
	notifier.SetSend(p.Send)
	if _, err := p.Run(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "sshdeck:", err)
	os.Exit(1)
}
