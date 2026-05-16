package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

type ProviderConfig struct {
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint"`
	APIKey   string `mapstructure:"api_key" yaml:"api_key"`
	Type     string `mapstructure:"type" yaml:"type,omitempty"` // "cloud" or "local"
}

type MCPServerConfig struct {
	Endpoint  string `mapstructure:"endpoint" yaml:"endpoint"`
	APIKey    string `mapstructure:"api_key" yaml:"api_key"`
	Transport string `mapstructure:"transport" yaml:"transport"` // "sse" or "stdio"
	Autostart bool   `mapstructure:"autostart" yaml:"autostart"`
}

type Config struct {
	DefaultProvider  string                      `mapstructure:"default_provider" yaml:"default_provider"`
	DefaultModel     string                      `mapstructure:"default_model" yaml:"default_model"`
	Providers        map[string]ProviderConfig   `mapstructure:"providers" yaml:"providers"`
	MCPServers       map[string]MCPServerConfig  `mapstructure:"mcp_servers" yaml:"mcp_servers,omitempty"`
	DiscoveredModels map[string][]string         `mapstructure:"discovered_models" yaml:"discovered_models,omitempty"`
	path             string
}

const defaultConfig = `# rig configuration
default_provider: openai
default_model: gpt-4o

providers:
  openai:
    endpoint: https://api.openai.com/v1
    api_key: "" # set your OpenAI API key here or via OPENAI_API_KEY env var

  ollama:
    endpoint: http://localhost:11434/v1
    api_key: "" # ollama doesn't need an API key

  anthropic:
    endpoint: https://api.anthropic.com
    api_key: "" # set your Anthropic API key here or via ANTHROPIC_API_KEY env var
`

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".rig")
}

func Load() (*Config, error) {
	dir := configDir()
	cfgFile := filepath.Join(dir, "config.yaml")

	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating config dir: %w", err)
		}
		if err := os.WriteFile(cfgFile, []byte(defaultConfig), 0o644); err != nil {
			return nil, fmt.Errorf("writing default config: %w", err)
		}
	}

	viper.SetConfigFile(cfgFile)
	viper.SetConfigType("yaml")

	// Env overrides: OPENAI_API_KEY, ANTHROPIC_API_KEY
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Apply env var overrides for API keys
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		if p, ok := cfg.Providers["openai"]; ok {
			p.APIKey = key
			cfg.Providers["openai"] = p
		}
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		if p, ok := cfg.Providers["anthropic"]; ok {
			p.APIKey = key
			cfg.Providers["anthropic"] = p
		}
	}

	cfg.path = cfgFile
	cfg.LoadModelsCache()
	return &cfg, nil
}

func (c *Config) Save() error {
	// Write discovered models to a separate cache file so we never
	// touch the user's config.yaml.
	cacheFile := filepath.Join(filepath.Dir(c.path), "models_cache.yaml")

	cache := struct {
		DiscoveredModels map[string][]string `yaml:"discovered_models"`
	}{DiscoveredModels: c.DiscoveredModels}

	data, err := yaml.Marshal(&cache)
	if err != nil {
		return fmt.Errorf("marshaling models cache: %w", err)
	}
	if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
		return fmt.Errorf("writing models cache: %w", err)
	}
	return nil
}

func (c *Config) LoadModelsCache() {
	cacheFile := filepath.Join(filepath.Dir(c.path), "models_cache.yaml")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return
	}
	var cache struct {
		DiscoveredModels map[string][]string `yaml:"discovered_models"`
	}
	if err := yaml.Unmarshal(data, &cache); err == nil && cache.DiscoveredModels != nil {
		c.DiscoveredModels = cache.DiscoveredModels
	}
}

func (c *Config) SaveConfig() error {
	if len(c.Providers) == 0 && c.DefaultProvider != "" {
		return fmt.Errorf("refusing to save config: providers map is empty (would lose all provider data)")
	}

	out := struct {
		DefaultProvider string                     `yaml:"default_provider"`
		DefaultModel    string                     `yaml:"default_model"`
		Providers       map[string]ProviderConfig  `yaml:"providers"`
		MCPServers      map[string]MCPServerConfig `yaml:"mcp_servers,omitempty"`
	}{
		DefaultProvider: c.DefaultProvider,
		DefaultModel:    c.DefaultModel,
		Providers:       c.Providers,
		MCPServers:      c.MCPServers,
	}

	data, err := yaml.Marshal(&out)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(c.path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func (c *Config) Path() string { return c.path }
