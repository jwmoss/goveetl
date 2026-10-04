package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDoAddsAuthAndDecodesJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(DefaultAuthHeader); got != "Bearer token-123" {
			t.Fatalf("auth header = %q", got)
		}
		if got := r.URL.Query().Get("page"); got != "1" {
			t.Fatalf("query page = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer server.Close()

	client := New(server.URL, WithAuth(DefaultAuthHeader, DefaultAuthScheme, "token-123"))
	query := url.Values{"page": []string{"1"}}
	var out map[string]string
	if err := client.DoJSON(context.Background(), http.MethodGet, "/test", query, nil, &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != "true" {
		t.Fatalf("decoded output = %#v", out)
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad token"}`))
	}))
	defer server.Close()

	client := New(server.URL)
	_, err := client.Do(context.Background(), http.MethodGet, "/private", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T", err)
	}
	if apiErr.Status != http.StatusUnauthorized || !strings.Contains(apiErr.Error(), "bad token") {
		t.Fatalf("api error = %v", apiErr)
	}
}

func TestDryRunBlocksMutations(t *testing.T) {
	client := New("https://example.invalid", WithDryRun(true), WithTimeout(time.Millisecond))
	if _, err := client.Do(context.Background(), http.MethodPost, "/mutate", nil, map[string]string{"x": "y"}); err == nil {
		t.Fatal("expected dry-run error")
	}
}
