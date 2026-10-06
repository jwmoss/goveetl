package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("base_url: https://file.example\ntoken: file-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvPrefix+"_BASE_URL", "https://env.example/")
	t.Setenv(EnvPrefix+"_TOKEN", "env-token")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://env.example" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Token != "env-token" {
		t.Fatalf("Token = %q", cfg.Token)
	}
}

func TestSaveWritesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	cfg := Config{BaseURL: "https://api.example.com", Token: "token"}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0600 {
		t.Fatalf("mode = %v", got)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0600 {
		t.Fatalf("updated config mode = %v", got)
	}
}

func TestAppHostDefaultsAndLegacyConfig(t *testing.T) {
	for _, body := range []string{"", "base_url: https://openapi.api.govee.com\n"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.BaseURL != "https://app2.govee.com" || cfg.OpenAPIBaseURL != "https://openapi.api.govee.com" {
			t.Errorf("app=%s official=%s", cfg.BaseURL, cfg.OpenAPIBaseURL)
		}
	}
}

func TestSavePrivateReplacement(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "config.yaml")
	original := []byte("original")
	if err := os.WriteFile(target, original, 0644); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := Save(path, Default()); err == nil {
		t.Error("Save followed symlink")
	}
	data, _ := os.ReadFile(target)
	if string(data) != string(original) {
		t.Error("symlink target changed")
	}
	info, _ := os.Stat(target)
	if info.Mode() != originalInfo.Mode() {
		t.Error("symlink target permissions changed")
	}
	_ = os.Remove(path)
	if err := os.Link(target, path); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(target)
	if string(data) != string(original) {
		t.Error("hardlink target changed")
	}
	info, _ = os.Stat(target)
	if info.Mode() != originalInfo.Mode() {
		t.Error("hardlink target permissions changed")
	}
}
