package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSettingsRoundTripKeepsComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	s := DefaultSettings()
	s.Prefix = "ctrl+]"
	s.SidebarWidth = 40
	s.Scrollback = 0
	s.Term = "screen-256color"
	s.ConnectTimeout = Duration(90 * time.Second)
	s.Keepalive = 0
	s.KnownHosts = "~/my hosts/known"
	s.UseAgent = false
	s.Identities = []string{"~/.ssh/work key", `C:\odd"name`}
	s.ConfirmClose = false
	s.RememberPasswords = false
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "# Key that starts a command") {
		t.Error("comments missing from saved file")
	}
	got, err := LoadSettings(path, false)
	if err != nil {
		t.Fatalf("load: %v\n%s", err, data)
	}
	if !reflect.DeepEqual(got, s) {
		t.Errorf("round trip:\n got %+v\nwant %+v", got, s)
	}

	s.Identities = nil
	SaveSettings(path, s)
	got, _ = LoadSettings(path, false)
	if len(got.Identities) != 0 {
		t.Errorf("empty identities came back as %q", got.Identities)
	}
}

func TestFirstStartWritesTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	if _, err := LoadSettings(path, true); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(path, false)
	if err != nil || !reflect.DeepEqual(got, DefaultSettings()) {
		t.Errorf("template does not load as defaults: %+v %v", got, err)
	}
}

func TestValidatePrefix(t *testing.T) {
	for _, ok := range []string{"ctrl+a", "ctrl+]", "alt+a", "ctrl+alt+x"} {
		if err := ValidatePrefix(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "a", "space", "enter", "ctrl+c", "ctrl+["} {
		if err := ValidatePrefix(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	path := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(path, []byte("prefix: x\n"), 0o600)
	if _, err := LoadSettings(path, false); err == nil {
		t.Error("bad prefix in file accepted")
	}
	if err := SaveSettings(path, Settings{Prefix: "q"}); err == nil {
		t.Error("bad prefix saved")
	}
}

func TestSettingsLanguage(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ lang, want, comment string }{
		{"uk", "uk", "# Налаштування sshdeck"},
		{"en", "en", "# sshdeck settings"},
		{"", "auto", ""},
		{"UK", "uk", ""},
	} {
		path := filepath.Join(dir, "c-"+tc.want+".yaml")
		s := DefaultSettings()
		s.Language = tc.lang
		if err := SaveSettings(path, s); err != nil {
			t.Fatalf("%q: %v", tc.lang, err)
		}
		got, err := LoadSettings(path, false)
		if err != nil || got.Language != tc.want {
			t.Errorf("%q: loaded %q, %v", tc.lang, got.Language, err)
		}
		raw, _ := os.ReadFile(path)
		if tc.comment != "" && !strings.HasPrefix(string(raw), tc.comment) {
			t.Errorf("%q: comments in the wrong language:\n%s", tc.lang, raw)
		}
	}
	s := DefaultSettings()
	s.Language = "de"
	if err := SaveSettings(filepath.Join(dir, "bad.yaml"), s); err == nil {
		t.Error("language de accepted")
	}
}

func TestSettingsTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	for in, want := range map[string]string{"": "terminal", "Mocha": "mocha", "frappé": "frappe", "latte": "latte"} {
		s := DefaultSettings()
		s.Theme = in
		if err := SaveSettings(path, s); err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		got, err := LoadSettings(path, false)
		if err != nil || got.Theme != want {
			t.Errorf("%q: loaded %q, %v", in, got.Theme, err)
		}
	}
	s := DefaultSettings()
	s.Theme = "dracula"
	if err := SaveSettings(path, s); err == nil {
		t.Error("theme dracula accepted")
	}
}

func TestSettingsLocalShell(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if !DefaultSettings().LocalShell {
		t.Fatal("the local entry should be on by default")
	}
	// an old config.yaml without the keys keeps the default
	os.WriteFile(path, []byte("prefix: ctrl+a\n"), 0o600)
	if s, err := LoadSettings(path, false); err != nil || !s.LocalShell {
		t.Fatalf("old file: %+v %v", s.LocalShell, err)
	}
	s := DefaultSettings()
	s.LocalShell, s.LocalCommand, s.LocalColor, s.LocalFill = false, "zsh -l", "Orange", "#ABC"
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(path, false)
	if err != nil || got.LocalShell || got.LocalCommand != "zsh -l" || got.LocalColor != "peach" || got.LocalFill != "#aabbcc" {
		t.Errorf("round trip: %+v %v", got, err)
	}
	s.LocalFill = "sparkly"
	if err := SaveSettings(path, s); err == nil {
		t.Error("bad local_fill accepted")
	}
}
