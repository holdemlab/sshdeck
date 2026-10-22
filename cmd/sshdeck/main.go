// Command sshdeck is a terminal SSH session manager with built-in tabs.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"

	"github.com/holdemlab/sshdeck/internal/config"
	"github.com/holdemlab/sshdeck/internal/i18n"
	"github.com/holdemlab/sshdeck/internal/ui"
)

// version is set at release time (-ldflags "-X main.version=v1.2.3"); for
// "go install …@v1.2.3" it comes from the module's build info.
var version = "dev"

func versionString() string {
	if version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
	}
	return version
}

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
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("sshdeck", versionString())
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
