package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jwmoss/goveetl/internal/config"
)

func TestAuthLogin(t *testing.T) {
	for _, name := range []string{"stdin", "environment", "verification-code", "verification-required", "wrong-password", "incomplete-session", "invalid-session", "missing-password", "missing-email", "redirect", "insecure-endpoint"} {
		t.Run(name, func(t *testing.T) {
			logins, checks, verificationRequests := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/account/rest/account/v2/login":
					logins++
					var body struct{ Email, Password, Client, Code string }
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					if r.Method != "POST" || body.Email != "test@example.com" || body.Password != "test-password" || body.Client != "test-client" || r.Header.Get("Authorization") != "" || r.Header.Get("clientType") != "1" {
						t.Error("incorrect login contract")
					}
					if name == "verification-code" && body.Code != "123456" {
						t.Error("verification code missing")
					}
					switch name {
					case "redirect":
						http.Redirect(w, r, "/credential-sink", http.StatusTemporaryRedirect)
					case "verification-required":
						_, _ = w.Write([]byte(`{"status":454,"message":"verification required"}`))
					case "wrong-password":
						_, _ = w.Write([]byte(`{"status":401,"message":"Incorrect password"}`))
					case "incomplete-session":
						_, _ = w.Write([]byte(`{"status":200,"client":{"token":"test-new-token"}}`))
					default:
						_, _ = w.Write([]byte(`{"status":200,"client":{"accountId":42,"token":"test-new-token","topic":"test-new-topic"}}`))
					}
				case "/account/rest/account/v1/verification":
					verificationRequests++
					var body struct {
						Email string
						Type  int
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Method != "POST" || body.Email != "test@example.com" || body.Type != 8 || r.Header.Get("Authorization") != "" {
						t.Error("incorrect email verification request")
					}
					_, _ = w.Write([]byte(`{"status":200}`))
				case "/bff-app/v1/device/list":
					checks++
					if r.Header.Get("Authorization") != "Bearer test-new-token" {
						t.Error("new session not verified")
					}
					if name == "invalid-session" {
						_, _ = w.Write([]byte(`{"status":401,"message":"token rejected"}`))
					} else {
						_, _ = w.Write([]byte(`{"status":200,"data":{"devices":[]}}`))
					}
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			for _, key := range []string{"TOKEN", "REFRESH_TOKEN", "EMAIL", "PASSWORD", "VERIFICATION_CODE", "CLIENT_ID", "ACCOUNT_ID", "ACCOUNT_TOPIC"} {
				t.Setenv("GOVEETL_"+key, "")
			}
			t.Setenv("GOVEETL_BASE_URL", server.URL)
			if name == "insecure-endpoint" {
				t.Setenv("GOVEETL_BASE_URL", "http://govee.invalid")
			}
			path := filepath.Join(t.TempDir(), "config.yaml")
			cfg := config.Default()
			cfg.APIKey, cfg.Token, cfg.RefreshToken = "test-api-key", "test-old-token", "test-old-refresh"
			cfg.AccountID, cfg.AccountTopic, cfg.ClientID = 7, "test-old-topic", "test-client"
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			args := []string{"--config", path, "--json", "--no-input", "auth", "login", "--email", "test@example.com"}
			input := ""
			if name == "stdin" {
				args = append(args, "--stdin")
				input = "test-password\n"
			} else if name != "missing-password" {
				t.Setenv("GOVEETL_PASSWORD", "test-password")
			}
			if name == "verification-code" {
				t.Setenv("GOVEETL_VERIFICATION_CODE", "123456")
			}
			if name == "missing-email" {
				args = args[:len(args)-2]
			}
			var out, errOut bytes.Buffer
			code := Execute(context.Background(), args, strings.NewReader(input), &out, &errOut)
			after, _ := os.ReadFile(path)
			if name == "stdin" || name == "environment" || name == "verification-code" {
				if code != exitOK {
					t.Fatalf("code=%d stderr=%s", code, &errOut)
				}
				saved, err := config.Load(path)
				if err != nil || saved.Token != "test-new-token" || saved.AccountID != 42 || saved.AccountTopic != "test-new-topic" || saved.RefreshToken != "" || saved.Email != "test@example.com" || saved.APIKey != "test-api-key" || checks != 1 || logins != 1 {
					t.Fatal("verified session not saved, or previous account data retained")
				}
				info, _ := os.Stat(path)
				if info.Mode().Perm() != 0600 {
					t.Fatal("session file is not private")
				}
			} else {
				if code == exitOK || !bytes.Equal(before, after) {
					t.Fatal("failed login changes the saved session")
				}
				if name == "verification-required" && (verificationRequests != 1 || !strings.Contains(errOut.String(), "GOVEETL_VERIFICATION_CODE")) {
					t.Fatal("missing verification instructions")
				}
				if (name == "missing-password" || name == "missing-email" || name == "insecure-endpoint") && logins != 0 {
					t.Fatal("missing credentials reach the server")
				}
			}
			for _, secret := range []string{"test-password", "test-new-token", "test-old-token", "test-old-refresh", "test-new-topic", "123456"} {
				if strings.Contains(out.String()+errOut.String(), secret) {
					t.Fatal("auth output leaks a credential")
				}
			}
		})
	}
}

func TestAuthRefresh(t *testing.T) {
	for _, name := range []string{"rotated", "rejected", "empty-token"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					RefreshToken string
					Type         int
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Method != "POST" || r.URL.Path != "/bff-app/v2/account/refresh-token" || body.RefreshToken != "test-refresh" || body.Type != 0 || r.Header.Get("Authorization") != "Bearer test-old-token" {
					t.Error("incorrect refresh contract")
				}
				switch name {
				case "rotated":
					_, _ = w.Write([]byte(`{"status":200,"data":{"token":"test-new-token","refreshToken":"test-new-refresh"}}`))
				case "rejected":
					_, _ = w.Write([]byte(`{"status":401,"message":"please login"}`))
				case "empty-token":
					_, _ = w.Write([]byte(`{"status":200,"data":{}}`))
				}
			}))
			defer server.Close()
			for _, key := range []string{"TOKEN", "REFRESH_TOKEN", "EMAIL", "CLIENT_ID", "ACCOUNT_ID", "ACCOUNT_TOPIC"} {
				t.Setenv("GOVEETL_"+key, "")
			}
			t.Setenv("GOVEETL_BASE_URL", server.URL)
			path := filepath.Join(t.TempDir(), "config.yaml")
			cfg := config.Default()
			cfg.Token, cfg.RefreshToken, cfg.ClientID = "test-old-token", "test-refresh", "test-client"
			cfg.AccountID, cfg.AccountTopic = 42, "test-private-topic"
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			var out, errOut bytes.Buffer
			code := Execute(context.Background(), []string{"--config", path, "--json", "auth", "refresh"}, strings.NewReader(""), &out, &errOut)
			if name == "rotated" {
				saved, err := config.Load(path)
				if code != exitOK || err != nil || saved.Token != "test-new-token" || saved.RefreshToken != "test-new-refresh" || saved.AccountID != 42 || saved.AccountTopic != "test-private-topic" {
					t.Fatal("refresh does not preserve account identity and rotate both tokens")
				}
			} else {
				after, _ := os.ReadFile(path)
				if code == exitOK || !bytes.Equal(before, after) {
					t.Fatal("failed refresh changes the saved session")
				}
				if name == "rejected" && !strings.Contains(errOut.String(), "auth login") {
					t.Fatal("refresh rejection has no recovery command")
				}
			}
			for _, secret := range []string{"test-old-token", "test-new-token", "test-new-refresh", "test-private-topic"} {
				if strings.Contains(out.String()+errOut.String(), secret) {
					t.Fatal("refresh output leaks a credential")
				}
			}
		})
	}
}
