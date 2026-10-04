package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBaseURL    = "https://openapi.api.govee.com"
	DefaultAuthHeader = "Authorization"
	DefaultAuthScheme = "Bearer"
	DefaultUserAgent  = "goveetl/dev"
)

type Client struct {
	baseURL    string
	token      string
	authHeader string
	authScheme string
	httpClient *http.Client
	userAgent  string
	trace      func(method, path string, status int, duration time.Duration)
	dryRun     bool
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.httpClient.Timeout = timeout
		}
	}
}

func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		if strings.TrimSpace(userAgent) != "" {
			c.userAgent = userAgent
		}
	}
}

func WithAuth(header, scheme, token string) Option {
	return func(c *Client) {
		if strings.TrimSpace(header) != "" {
			c.authHeader = header
		}
		c.authScheme = strings.TrimSpace(scheme)
		c.token = strings.TrimSpace(token)
	}
}

func WithTrace(trace func(method, path string, status int, duration time.Duration)) Option {
	return func(c *Client) {
		c.trace = trace
	}
}

func WithDryRun(dryRun bool) Option {
	return func(c *Client) {
		c.dryRun = dryRun
	}
}

func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		authHeader: DefaultAuthHeader,
		authScheme: DefaultAuthScheme,
		userAgent:  DefaultUserAgent,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type APIError struct {
	Status  int
	Method  string
	Path    string
	Body    []byte
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Message)
	}
	return fmt.Sprintf("%s %s: HTTP %d", e.Method, e.Path, e.Status)
}

func (c *Client) Do(ctx context.Context, method, requestPath string, query url.Values, body any) ([]byte, error) {
	return c.DoWithHeaders(ctx, method, requestPath, query, body, nil)
}

func (c *Client) DoWithHeaders(ctx context.Context, method, requestPath string, query url.Values, body any, extra http.Header) ([]byte, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return nil, fmt.Errorf("method is required")
	}
	if c.dryRun && method != http.MethodGet {
		return nil, fmt.Errorf("dry-run: refusing %s %s", method, requestPath)
	}

	endpoint, err := c.url(requestPath, query)
	if err != nil {
		return nil, err
	}

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	for key, values := range extra {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" && c.authHeader != "" {
		value := c.token
		if c.authScheme != "" {
			value = c.authScheme + " " + c.token
		}
		req.Header.Set(c.authHeader, value)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if c.trace != nil {
		c.trace(method, req.URL.Path, resp.StatusCode, time.Since(start))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return data, &APIError{
			Status:  resp.StatusCode,
			Method:  method,
			Path:    req.URL.Path,
			Body:    data,
			Message: extractErrorMessage(data),
		}
	}
	return data, nil
}

func (c *Client) DoJSON(ctx context.Context, method, requestPath string, query url.Values, body any, out any) error {
	return c.DoJSONWithHeaders(ctx, method, requestPath, query, body, out, nil)
}

func (c *Client) DoJSONWithHeaders(ctx context.Context, method, requestPath string, query url.Values, body any, out any, extra http.Header) error {
	data, err := c.DoWithHeaders(ctx, method, requestPath, query, body, extra)
	if err != nil {
		return err
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response JSON: %w", err)
	}
	return nil
}

// BaseURL exposes the configured base URL for cross-service calls.
func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) url(requestPath string, query url.Values) (string, error) {
	if strings.HasPrefix(requestPath, "http://") || strings.HasPrefix(requestPath, "https://") {
		u, err := url.Parse(requestPath)
		if err != nil {
			return "", fmt.Errorf("parse URL: %w", err)
		}
		if len(query) > 0 {
			u.RawQuery = mergeQuery(u.Query(), query).Encode()
		}
		return u.String(), nil
	}
	if c.baseURL == "" {
		return "", fmt.Errorf("base URL is required")
	}
	joined := c.baseURL + "/" + strings.TrimLeft(requestPath, "/")
	u, err := url.Parse(joined)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}
	if len(query) > 0 {
		u.RawQuery = mergeQuery(u.Query(), query).Encode()
	}
	return u.String(), nil
}

func mergeQuery(dst, src url.Values) url.Values {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
	return dst
}

func extractErrorMessage(data []byte) string {
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		return strings.TrimSpace(string(data))
	}
	for _, key := range []string{"message", "error", "detail"} {
		if value, ok := generic[key].(string); ok {
			return value
		}
	}
	if errorsValue, ok := generic["errors"]; ok {
		return fmt.Sprint(errorsValue)
	}
	return strings.TrimSpace(string(data))
}

type Resource struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	var resources []Resource
	if err := c.DoJSON(ctx, http.MethodGet, "/v1/user/devices", nil, nil, &resources); err != nil {
		return nil, err
	}
	return resources, nil
}

func (c *Client) GetResource(ctx context.Context, id string) (*Resource, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("resource id is required")
	}
	var resource Resource
	path := strings.TrimRight("/v1/user/devices", "/") + "/" + url.PathEscape(id)
	if err := c.DoJSON(ctx, http.MethodGet, path, nil, nil, &resource); err != nil {
		return nil, err
	}
	return &resource, nil
}
