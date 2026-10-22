package config

import (
	"os"
	"testing"
)

// The comments in config.yaml follow the language, and "auto" reads it from
// the locale: pin English so the tests pass whatever $LANG the machine has.
func TestMain(m *testing.M) {
	os.Setenv("LC_ALL", "C")
	os.Setenv("LC_MESSAGES", "")
	os.Setenv("LANG", "C")
	os.Exit(m.Run())
}
