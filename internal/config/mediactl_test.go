package config

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLoadAndSaveMediaCtlPreference(t *testing.T) {
	SetXDGConfig(true)
	t.Cleanup(func() { SetXDGConfig(false) })
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := DefaultConfig()
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	path, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		json string
		want bool
	}{
		{name: "old configuration", json: `{}`, want: true},
		{name: "empty mediactl section", json: `{"mediactl":{}}`, want: true},
		{name: "enabled", json: `{"mediactl":{"enabled":true}}`, want: true},
		{name: "disabled", json: `{"mediactl":{"enabled":false}}`, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.json), 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if err := loaded.Save(); err != nil {
				t.Fatal(err)
			}
			// #nosec G304 -- path is this test's config file under t.TempDir().
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var stored struct {
				MediaCtl *struct {
					Enabled bool `json:"enabled"`
				} `json:"mediactl"`
			}
			if err := json.Unmarshal(data, &stored); err != nil {
				t.Fatal(err)
			}
			if stored.MediaCtl == nil || stored.MediaCtl.Enabled != tc.want {
				t.Fatalf("saved mediactl = %+v, want enabled=%v", stored.MediaCtl, tc.want)
			}
		})
	}
}
