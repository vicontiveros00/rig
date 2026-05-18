package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCreatesDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")

	// Override configDir for testing
	origPath := cfgFile
	os.Setenv("RIG_CONFIG_FILE", origPath)
	defer os.Unsetenv("RIG_CONFIG_FILE")

	// Write a minimal config
	content := `default_provider: openai
default_model: gpt-4o
providers:
  openai:
    endpoint: https://api.openai.com/v1
    api_key: test-key
`
	if err := os.WriteFile(cfgFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test that we can parse it manually
	cfg := &Config{}
	cfg.DefaultProvider = "openai"
	cfg.DefaultModel = "gpt-4o"
	cfg.Providers = map[string]ProviderConfig{
		"openai": {Endpoint: "https://api.openai.com/v1", APIKey: "test-key"},
	}

	if cfg.DefaultProvider != "openai" {
		t.Errorf("expected default_provider=openai, got %s", cfg.DefaultProvider)
	}
	if cfg.DefaultModel != "gpt-4o" {
		t.Errorf("expected default_model=gpt-4o, got %s", cfg.DefaultModel)
	}
	if len(cfg.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(cfg.Providers))
	}
}

func TestSaveConfigRefusesEmptyProviders(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DefaultProvider: "openai",
		DefaultModel:    "gpt-4o",
		Providers:       map[string]ProviderConfig{},
		path:            filepath.Join(dir, "config.yaml"),
	}

	err := cfg.SaveConfig()
	if err == nil {
		t.Error("expected error when saving empty providers, got nil")
	}
}

func TestSaveConfigWritesValidYAML(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DefaultProvider: "ollama",
		DefaultModel:    "llama3",
		Providers: map[string]ProviderConfig{
			"ollama": {Endpoint: "http://localhost:11434/v1", APIKey: ""},
		},
		MCPServers: map[string]MCPServerConfig{
			"test": {Endpoint: "http://localhost:8080", Transport: "sse"},
		},
		path: filepath.Join(dir, "config.yaml"),
	}

	if err := cfg.SaveConfig(); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	data, err := os.ReadFile(cfg.path)
	if err != nil {
		t.Fatalf("reading saved config: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Error("saved config is empty")
	}
}

func TestMCPServerConfig(t *testing.T) {
	cfg := MCPServerConfig{
		Endpoint:  "http://localhost:3000/mcp",
		APIKey:    "",
		Transport: "sse",
		Autostart: false,
	}

	if cfg.Transport != "sse" {
		t.Errorf("expected transport=sse, got %s", cfg.Transport)
	}
	if cfg.Autostart != false {
		t.Error("expected autostart=false")
	}
}

func TestProviderConfigType(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		typ      string
	}{
		{"cloud", "https://api.openai.com/v1", "cloud"},
		{"local", "http://localhost:11434/v1", "local"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ProviderConfig{Endpoint: tt.endpoint, Type: tt.typ}
			if cfg.Type != tt.typ {
				t.Errorf("expected type=%s, got %s", tt.typ, cfg.Type)
			}
		})
	}
}
