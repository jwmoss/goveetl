package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDoAddsAuthAndDecodesJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("auth header = %q", got)
		}
		if got := r.URL.Query().Get("page"); got != "1" {
			t.Fatalf("query page = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer server.Close()

	client := New(server.URL, WithAuth("Authorization", "Bearer", "token-123"))
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

type failingTransport func(*http.Request) (*http.Response, error)

func (f failingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCredentialSafety(t *testing.T) {
	const secret = "fixture-secret"

	transport := failingTransport(func(r *http.Request) (*http.Response, error) {
		status, body, headers := 200, "{}", make(http.Header)
		if r.URL.Host == "sink.invalid" || r.URL.Port() == "81" || r.URL.Scheme == "https" {
			if r.Header.Get("X-Key") != "" || r.Header.Get("X-Session") != "" {
				t.Error("credentials crossed origin")
			}
		} else if r.URL.Path == "/redirect" {
			status = 302
			headers.Set("Location", "http://sink.invalid/")
		} else {
			if r.Header.Get("X-Key") != secret {
				t.Error("same-origin credential missing")
			}
			status = 401
			body = fmt.Sprintf(`{"message":%q}`, secret)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: headers, Request: r}, nil
	})
	c := New("http://origin.invalid", WithHTTPClient(&http.Client{Transport: transport}), WithAuth("X-Key", "", secret))
	for _, target := range []string{"http://sink.invalid/", "http://origin.invalid:81/", "https://origin.invalid/"} {
		if _, err := c.DoWithHeaders(context.Background(), "GET", target, nil, nil, http.Header{"X-Session": {secret}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Do(context.Background(), "GET", "/redirect", nil, nil); err == nil {
		t.Error("cross-origin redirect succeeded")
	}
	_, err := c.Do(context.Background(), "GET", "/"+secret, nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || strings.Contains(err.Error(), secret) || strings.Contains(string(apiErr.Body), secret) {
		t.Errorf("unsafe API error: %v", err)
	}
	sentinel := errors.New("transport echoed " + secret)
	supplied := &http.Client{Transport: failingTransport(func(*http.Request) (*http.Response, error) { return nil, sentinel })}
	c = New("http://origin.invalid", WithHTTPClient(supplied), WithTimeout(time.Second), WithAuth("X-Key", "", secret))
	_, err = c.Do(context.Background(), "GET", "/", nil, nil)
	if err == nil || strings.Contains(err.Error(), secret) || !errors.Is(err, sentinel) {
		t.Errorf("unsafe transport error: %v", err)
	}
	if supplied.Timeout != 0 || supplied.CheckRedirect != nil {
		t.Error("supplied HTTP client changed")
	}
}

func TestRawBodyExact(t *testing.T) {
	const exact = `{ "id":9007199254740993, "id":1e100 }`
	c := New("http://fixture.invalid", WithHTTPClient(&http.Client{Transport: failingTransport(func(r *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(r.Body)
		if string(data) != exact {
			t.Errorf("body = %s", data)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	})}))
	if _, err := c.Do(context.Background(), "POST", "/", nil, json.RawMessage(exact)); err != nil {
		t.Fatal(err)
	}
}

func TestResponseLimit(t *testing.T) {
	const limit = 64 << 20
	data := strings.Repeat("x", limit+1)
	c := New("http://fixture.invalid", WithHTTPClient(&http.Client{Transport: failingTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(data)), Header: make(http.Header), Request: r}, nil
	})}))
	got, err := c.Do(context.Background(), "GET", "/", nil, nil)
	if err == nil || got != nil || !strings.Contains(err.Error(), "64 MiB") {
		t.Fatalf("response bytes=%d error=%v", len(got), err)
	}
}
