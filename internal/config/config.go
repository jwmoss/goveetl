package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	AppName           = "goveetl"
	EnvPrefix        = "GOVEETL"
	DefaultBaseURL    = "https://openapi.api.govee.com"
	DefaultAuthHeader = "Govee-API-Key"
	DefaultAuthScheme = ""
	ConfigFilename    = "config.yaml"
)

type Config struct {
	BaseURL    string `yaml:"base_url"`
	Token      string `yaml:"token,omitempty"`
	AuthHeader string `yaml:"auth_header,omitempty"`
	AuthScheme string `yaml:"auth_scheme,omitempty"`
}

func Default() Config {
	return Config{
		BaseURL:    DefaultBaseURL,
		AuthHeader: DefaultAuthHeader,
		AuthScheme: DefaultAuthScheme,
	}
}

func DefaultPath() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, AppName, ConfigFilename)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ConfigFilename)
	}
	return filepath.Join(home, ".config", AppName, ConfigFilename)
}

func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		path = DefaultPath()
	}
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config file %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	applyEnv(&cfg)
	normalize(&cfg)
	return &cfg, nil
}

func Save(path string, cfg Config) error {
	if path == "" {
		path = DefaultPath()
	}
	normalize(&cfg)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("base URL is required; set --base-url, %s_BASE_URL, or config file base_url", EnvPrefix)
	}
	return nil
}

func (c Config) Redacted() map[string]string {
	token := ""
	if c.Token != "" {
		token = "redacted"
	}
	return map[string]string{
		"base_url":    c.BaseURL,
		"token":       token,
		"auth_header": c.AuthHeader,
		"auth_scheme": c.AuthScheme,
	}
}

func applyEnv(cfg *Config) {
	if value := os.Getenv(EnvPrefix + "_BASE_URL"); value != "" {
		cfg.BaseURL = value
	}
	if value := os.Getenv(EnvPrefix + "_TOKEN"); value != "" {
		cfg.Token = value
	}
	if value := os.Getenv(EnvPrefix + "_AUTH_HEADER"); value != "" {
		cfg.AuthHeader = value
	}
	if value := os.Getenv(EnvPrefix + "_AUTH_SCHEME"); value != "" {
		cfg.AuthScheme = value
	}
}

func normalize(cfg *Config) {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.AuthHeader = strings.TrimSpace(cfg.AuthHeader)
	cfg.AuthScheme = strings.TrimSpace(cfg.AuthScheme)
	if cfg.AuthHeader == "" {
		cfg.AuthHeader = DefaultAuthHeader
	}
	if cfg.AuthScheme == "" {
		cfg.AuthScheme = DefaultAuthScheme
	}
}
