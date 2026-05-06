package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveOverwritesExistingFileInPlace(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	if err := os.WriteFile(configPath, []byte("{\n  \"daemons\": {\n    \"heating_enabled\": false\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := &Config{}
	cfg.SetHeatingDaemonEnabled(true)

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reloaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !reloaded.HeatingDaemonEnabled() {
		t.Fatal("expected rewritten config file to contain updated heating daemon state")
	}
	if reloaded.HotWaterDaemonEnabled() != true {
		t.Fatal("expected missing hot-water flag to keep default enabled behavior after rewrite")
	}
}
