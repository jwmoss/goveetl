package tests

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cli-tests-*")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "cli.exe")
	build := exec.Command("go", "build", "-o", binary, "../cmd/goveetl")
	if output, err := build.CombinedOutput(); err != nil {
		_ = os.RemoveAll(dir)
		panic(fmt.Sprintf("build CLI: %v\n%s", err, output))
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func run(t *testing.T, path, input string, env []string, args ...string) (int, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, append([]string{"--config", path}, args...)...)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GOVEETL_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "NO_COLOR=1", "GOVEETL_BASE_URL=http://127.0.0.1:1", "GOVEETL_OPENAPI_BASE_URL=http://127.0.0.1:1", "GOVEETL_DEVICE_BASE_URL=http://127.0.0.1:1")
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	if ctx.Err() != nil {
		t.Fatal("CLI exceeded its deadline")
	}
	return code, stdout.String(), stderr.String()
}

func TestLocalContracts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	code, stdout, stderr := run(t, path, "dummy-token\n", nil, "config", "init", "--token-stdin", "--json")
	if code != 0 || !json.Valid([]byte(stdout)) || stderr != "" || strings.Contains(stdout, "dummy-token") {
		t.Fatalf("config init: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, path, "", nil, "config", "show", "--json")
	var cfg map[string]string
	if err := json.Unmarshal([]byte(stdout), &cfg); err != nil || code != 0 || cfg["path"] != path || cfg["token"] != "redacted" {
		t.Fatalf("config show: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if err := os.WriteFile(path, []byte("token: [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--version"}, {"version", "--json"}, {"--help"}, {"completion", "bash"},
	} {
		code, stdout, stderr = run(t, path, "", nil, args...)
		if code != 0 || stdout == "" || stderr != "" {
			t.Errorf("%v depends on config: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func TestUsageExitCodes(t *testing.T) {
	for _, args := range [][]string{
		{"--unknown"}, {"unknown-command"}, {"version", "extra"},
		{"devices", "unknown-command"}, {"config", "unknown-command"},
		{"config", "show", "extra"}, {"config", "init", "extra"},
		{"auth", "refresh", "extra"}, {"mqtt", "cert", "extra"},
		{"config", "set", "account_id", "bad-number"},
		{"mqtt", "topic", "--device", "test"},
		{"raw", "GET", "/", "--data", ""},
		{"raw", "GET", "/", "--data", "{}", "--file", "missing"},
		{"raw", "POST", "/", "--data", "{"},
		{"version", "--json", "--plain"}, {"config", "show", "--json", "--plain"},
		{"doctor", "--timeout", "0s", "--base-url", "http://127.0.0.1:1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, stdout, stderr := run(t, filepath.Join(t.TempDir(), "config.yaml"), "", nil, args...)
			if code != 2 || stdout != "" || stderr == "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestConfigWriteSafety(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	code, _, _ := run(t, path, "", nil, "--dry-run", "config", "init")
	if _, err := os.Stat(path); !os.IsNotExist(err) || code == 0 {
		t.Fatal("dry-run created a config file or reported success")
	}
	original := []byte("base_url: https://example.invalid\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"config", "init"}, {"--dry-run", "config", "init", "--force"},
	} {
		code, _, _ = run(t, path, "", nil, args...)
		data, err := os.ReadFile(path)
		if err != nil || code == 0 || !bytes.Equal(data, original) {
			t.Fatalf("%v changed existing config", args)
		}
	}
	code, _, stderr := run(t, path, "new-dummy-token", nil, "config", "init", "--force", "--token-stdin")
	if code != 0 {
		t.Fatal(stderr)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("config mode = %o", info.Mode().Perm())
	}
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(path, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	before, _ := os.ReadFile(path)
	code, _, _ = run(t, link, "", nil, "config", "init", "--force")
	after, _ := os.ReadFile(path)
	if code == 0 || !bytes.Equal(before, after) {
		t.Fatal("config init followed a destination symlink")
	}
}

func TestCredentialBoundaries(t *testing.T) {
	const token = "dummy-credential"
	captured := make(chan http.Header, 8)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- r.Header.Clone()
		_, _ = io.WriteString(w, `{}`)
	}))
	defer sink.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, sink.URL, http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintf(w, `{"message":"invalid credential %s"}`, token)
	}))
	defer origin.Close()
	path := filepath.Join(t.TempDir(), "config.yaml")
	for _, header := range []string{"Authorization", "X-API-Key"} {
		env := []string{"GOVEETL_TOKEN=" + token, "GOVEETL_AUTH_HEADER=" + header}
		code, _, stderr := run(t, path, "", env, "--base-url", origin.URL, "raw", "GET", sink.URL)
		if code != 0 {
			t.Fatal(stderr)
		}
		if got := (<-captured).Get(header); got != "" {
			t.Errorf("off-origin %s = %q", header, got)
		}
		code, _, _ = run(t, path, "", env, "--base-url", origin.URL, "raw", "GET", "/redirect")
		if code == 0 {
			t.Error("cross-origin redirect succeeded")
		}
		select {
		case <-captured:
			t.Error("redirect reached another origin")
		default:
		}
		code, stdout, stderr := run(t, path, "", env, "--base-url", origin.URL, "raw", "GET", "/error")
		if code != 1 || stdout != "" || strings.Contains(stderr, token) {
			t.Errorf("API error: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
	}
}

func TestRawJSONContract(t *testing.T) {
	const exact = `{ "id":9007199254740993, "exponent":1e100, "id":9007199254740993 }`
	bodies := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			bodies <- string(body)
		}
		if r.URL.Path == "/text" {
			_, _ = io.WriteString(w, "not JSON")
			return
		}
		_, _ = io.WriteString(w, exact)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config.yaml")
	code, stdout, stderr := run(t, path, "", nil, "--base-url", server.URL, "raw", "POST", "/", "--data", exact, "--json")
	if code != 0 || stdout != exact+"\n" || <-bodies != exact {
		t.Fatalf("JSON precision: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, path, "", nil, "--base-url", server.URL, "raw", "GET", "/text", "--json")
	if code != 1 || stdout != "" || stderr == "" {
		t.Fatalf("non-JSON response: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestDryRunMutations(t *testing.T) {
	env := []string{"GOVEETL_BASE_URL=http://127.0.0.1:1", "GOVEETL_OPENAPI_BASE_URL=http://127.0.0.1:1", "GOVEETL_DEVICE_BASE_URL=http://127.0.0.1:1", "GOVEETL_TOKEN=fixture-token", "GOVEETL_API_KEY=fixture-key", "GOVEETL_PASSWORD=fixture-password"}
	for _, args := range [][]string{
		{"control", "test:H6006", "brightness", "20"},
		{"control", "test:H6006", "turn", `{"val":1}`, "--backend", "mqtt"},
		{"groups", "control", "42", "1"},
		{"lan", "control", "127.0.0.1", "turn", `{"value":1}`},
		{"auth", "login", "--email", "fixture@example.invalid", "--no-input"},
		{"auth", "refresh"}, {"auth", "logout"},
		{"config", "set", "token", "--stdin"}, {"config", "init", "--force"},
		{"automations", "set", "42", "test:H706C", "--brightness", "20"},
		{"automations", "remove-device", "42", "test:H616C"},
		{"raw", "POST", "/mutate", "--data", "{}"},
		{"raw", "POST", "/mutate", "--backend", "app", "--data", "{}"},
	} {
		t.Run(strings.Join(args[:2], " "), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			original := []byte("email: fixture@example.invalid\n")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			code, stdout, stderr := run(t, path, "fixture-new-token", env, append([]string{"--dry-run"}, args...)...)
			after, err := os.ReadFile(path)
			if code != 1 || stdout != "" || !strings.Contains(stderr, "dry-run:") || err != nil || !bytes.Equal(original, after) {
				t.Fatalf("code=%d stdout=%q stderr=%q config error=%v", code, stdout, stderr, err)
			}
		})
	}
}

func TestVersionIgnoresConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("token: [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, path, "", nil, "--version", "--json")
	if code != 0 || !json.Valid([]byte(stdout)) || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestBuildMetadataFallback(t *testing.T) {
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"version": "dev", "commit": "unknown", "date": "unknown"}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		want["version"] = info.Main.Version
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			want["commit"] = setting.Value
		}
		if setting.Key == "vcs.time" {
			want["date"] = setting.Value
		}
	}
	code, stdout, stderr := run(t, filepath.Join(t.TempDir(), "config.yaml"), "", nil, "version", "--json")
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || code != 0 || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s=%q want %q", key, got[key], value)
		}
	}
}
func TestSelectedConfigPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selected.yaml")
	for _, flags := range [][]string{nil, {"--json"}} {
		code, stdout, stderr := run(t, path, "", nil, append([]string{"config", "show"}, flags...)...)
		if code != 0 || !strings.Contains(stdout, path) {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
	}
}
