package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Test that defaults are applied, especially for time.Duration defaults.
func TestDefaultsAndDuration(t *testing.T) {
	type AppCfg struct {
		Server struct {
			ReadTimeout time.Duration `yaml:"read_timeout" env:"READ_TIMEOUT" default:"5s"`
		} `yaml:"server"`
	}

	var cfg AppCfg
	if err := New().Load(&cfg); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.ReadTimeout != 5*time.Second {
		t.Fatalf("expected ReadTimeout 5s, got %v", cfg.Server.ReadTimeout)
	}
}

// Test YAML loading and that environment variables override YAML values.
// Uses env source without prefix for clarity in tests.
func TestYAMLAndEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "config.yml")

	yamlContent := `server:
  port: "8081"
  read_timeout: "2s"
`
	if err := os.WriteFile(fpath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write yaml: %v", err)
	}

	// Ensure environment variable overrides YAML
	if err := os.Setenv("SERVER__PORT", "9090"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	defer os.Unsetenv("SERVER__PORT")

	var cfg struct {
		Server struct {
			Port        string        `yaml:"port" env:"PORT" default:"8081"`
			ReadTimeout time.Duration `yaml:"read_timeout" env:"READ_TIMEOUT" default:"10s"`
		} `yaml:"server"`
	}

	loader := New().
		AddSource(NewYAMLSource(fpath)).
		AddSource(NewEnvSource(""))

	if err := loader.Load(&cfg); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != "9090" {
		t.Fatalf("expected Port overridden by ENV to 9090, got %q", cfg.Server.Port)
	}

	if cfg.Server.ReadTimeout != 2*time.Second {
		t.Fatalf("expected ReadTimeout from YAML 2s, got %v", cfg.Server.ReadTimeout)
	}
}

// Ensure LoadFromSources variadic helper works and accepts multiple sources.
func TestLoadFromSourcesVariadic(t *testing.T) {
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "config.yml")

	yamlContent := `server:
  port: "7070"
`
	if err := os.WriteFile(fpath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write yaml: %v", err)
	}

	var cfg struct {
		Server struct {
			Port string `yaml:"port" env:"PORT" default:"8081"`
		} `yaml:"server"`
	}

	// Use LoadFromSources helper which should accept variadic sources.
	if err := LoadFromSources(&cfg, NewYAMLSource(fpath), NewEnvSource("")); err != nil {
		t.Fatalf("LoadFromSources failed: %v", err)
	}

	if cfg.Server.Port != "7070" {
		t.Fatalf("expected Port from YAML 7070, got %q", cfg.Server.Port)
	}
}
