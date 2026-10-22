package ui

import (
	"os"
	"testing"
)

// Tests expect the English interface unless they switch it themselves.
func TestMain(m *testing.M) {
	os.Setenv("LC_ALL", "C")
	os.Setenv("LANG", "C")
	os.Exit(m.Run())
}
