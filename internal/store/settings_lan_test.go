package store

import (
	"github.com/rockbenben/launchbay/internal/i18n"
	"os"
	"strings"
	"testing"
)

func TestLanguagePreservesLANRules(t *testing.T) {
	s := New(t.TempDir())
	if err := s.WriteFileJSON(s.SettingsPath(), Settings{LANHosts: []string{"nas.example.com"}, LANCIDRs: []string{"10.0.0.0/8"}}); err != nil {
		t.Fatal(err)
	}
	for _, write := range []func() error{func() error { return s.SetLanguage(i18n.En) }, s.ClearLanguage} {
		if err := write(); err != nil {
			t.Fatal(err)
		}
		st, err := s.readSettings()
		if err != nil || strings.Join(s.LANHosts(), ",") != "nas.example.com" || strings.Join(st.LANCIDRs, ",") != "10.0.0.0/8" {
			t.Fatal(st, err)
		}
	}
	broken := []byte(`{"language":`)
	if err := os.WriteFile(s.SettingsPath(), broken, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLanguage(i18n.En); err == nil {
		t.Fatal("overwrote broken config")
	}
	raw, _ := os.ReadFile(s.SettingsPath())
	if string(raw) != string(broken) {
		t.Fatal(string(raw))
	}
}
