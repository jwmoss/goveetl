package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	AppName           = "goveetl"
	EnvPrefix         = "GOVEETL"
	DefaultBaseURL    = "https://app2.govee.com"
	DefaultAuthHeader = "Authorization"
	DefaultAuthScheme = "Bearer"
	ConfigFilename    = "config.yaml"
)

type Config struct {
	BaseURL        string `yaml:"base_url"`
	OpenAPIBaseURL string `yaml:"openapi_base_url,omitempty"`
	DeviceBaseURL  string `yaml:"device_base_url,omitempty"`
	APIKey         string `yaml:"api_key,omitempty"`
	Token          string `yaml:"token,omitempty"`
	RefreshToken   string `yaml:"refresh_token,omitempty"`
	AccountTopic   string `yaml:"account_topic,omitempty"`
	AccountID      int    `yaml:"account_id,omitempty"`
	ClientID       string `yaml:"client_id,omitempty"`
	Email          string `yaml:"email,omitempty"`
	IotVersion     string `yaml:"iot_version,omitempty"`
	LANKey         string `yaml:"lan_key,omitempty"`
	AuthHeader     string `yaml:"auth_header,omitempty"`
	AuthScheme     string `yaml:"auth_scheme,omitempty"`
}

func Default() Config {
	return Config{
		BaseURL:        DefaultBaseURL,
		OpenAPIBaseURL: "https://openapi.api.govee.com",
		DeviceBaseURL:  "https://device.govee.com",
		AuthHeader:     DefaultAuthHeader,
		AuthScheme:     DefaultAuthScheme,
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

	// v1.0.0 saved the official host as the app host. Keep explicit env overrides.
	if strings.TrimRight(cfg.BaseURL, "/") == "https://openapi.api.govee.com" {
		cfg.BaseURL = DefaultBaseURL
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
	redact := func(value string) string {
		if value == "" {
			return ""
		}
		return "redacted"
	}
	return map[string]string{
		"base_url":         c.BaseURL,
		"openapi_base_url": c.OpenAPIBaseURL,
		"device_base_url":  c.DeviceBaseURL,
		"api_key":          redact(c.APIKey),
		"token":            redact(c.Token),
		"refresh_token":    redact(c.RefreshToken),
		"account_topic":    c.AccountTopic,
		"account_id":       fmt.Sprint(c.AccountID),
		"client_id":        c.ClientID,
		"email":            c.Email,
		"iot_version":      c.IotVersion,
		"lan_key":          redact(c.LANKey),
		"auth_header":      c.AuthHeader,
		"auth_scheme":      c.AuthScheme,
	}
}

func applyEnv(cfg *Config) {
	if value := os.Getenv(EnvPrefix + "_BASE_URL"); value != "" {
		cfg.BaseURL = value
	}
	if value := os.Getenv(EnvPrefix + "_OPENAPI_BASE_URL"); value != "" {
		cfg.OpenAPIBaseURL = value
	}
	if value := os.Getenv(EnvPrefix + "_DEVICE_BASE_URL"); value != "" {
		cfg.DeviceBaseURL = value
	}
	if value := os.Getenv(EnvPrefix + "_API_KEY"); value != "" {
		cfg.APIKey = value
	}
	if value := os.Getenv(EnvPrefix + "_TOKEN"); value != "" {
		cfg.Token = value
	}
	if value := os.Getenv(EnvPrefix + "_REFRESH_TOKEN"); value != "" {
		cfg.RefreshToken = value
	}
	if value := os.Getenv(EnvPrefix + "_ACCOUNT_TOPIC"); value != "" {
		cfg.AccountTopic = value
	}
	if value := os.Getenv(EnvPrefix + "_ACCOUNT_ID"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.AccountID = parsed
		}
	}
	if value := os.Getenv(EnvPrefix + "_CLIENT_ID"); value != "" {
		cfg.ClientID = value
	}
	if value := os.Getenv(EnvPrefix + "_EMAIL"); value != "" {
		cfg.Email = value
	}
	if value := os.Getenv(EnvPrefix + "_IOT_VERSION"); value != "" {
		cfg.IotVersion = value
	}
	if value := os.Getenv(EnvPrefix + "_LAN_KEY"); value != "" {
		cfg.LANKey = value
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
	cfg.OpenAPIBaseURL = strings.TrimRight(strings.TrimSpace(cfg.OpenAPIBaseURL), "/")
	cfg.DeviceBaseURL = strings.TrimRight(strings.TrimSpace(cfg.DeviceBaseURL), "/")
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.RefreshToken = strings.TrimSpace(cfg.RefreshToken)
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.IotVersion = strings.TrimSpace(cfg.IotVersion)
	cfg.AuthHeader = strings.TrimSpace(cfg.AuthHeader)
	cfg.AuthScheme = strings.TrimSpace(cfg.AuthScheme)
	if cfg.AuthHeader == "" {
		cfg.AuthHeader = DefaultAuthHeader
	}
	if cfg.AuthScheme == "" {
		cfg.AuthScheme = DefaultAuthScheme
	}
}
