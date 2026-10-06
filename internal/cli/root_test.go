package cli

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"

	"github.com/jwmoss/goveetl/internal/config"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProviderTransportOptions(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	cases := []struct {
		args []string
		body string
	}{
		{[]string{"devices", "list", "--backend", "app"}, `{"status":200,"data":{"devices":[]}}`},
		{[]string{"devices", "list"}, `{"code":200,"data":[]}`},
		{[]string{"devices", "state", "test:H6006"}, `{"code":200,"payload":{"device":"test","sku":"H6006"}}`},
		{[]string{"auth", "login", "--email", "fixture@example.invalid", "--no-input"}, `{"status":401,"message":"password rejected"}`},
		{[]string{"mqtt", "topic", "--device", "test", "--sku", "H6006"}, `{"status":200,"data":{"endpoint":"fixture"},"topic":"fixture-topic"}`},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Setenv("GOVEETL_BASE_URL", "http://localhost")
			t.Setenv("GOVEETL_OPENAPI_BASE_URL", "http://official.invalid")
			t.Setenv("GOVEETL_DEVICE_BASE_URL", "http://device.invalid")
			t.Setenv("GOVEETL_TOKEN", "fixture-token")
			t.Setenv("GOVEETL_API_KEY", "fixture-key")
			t.Setenv("GOVEETL_CLIENT_ID", "fixture-client")
			t.Setenv("GOVEETL_PASSWORD", "fixture-password")
			http.DefaultTransport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 100*time.Millisecond {
					t.Errorf("timeout not applied: %v %v", deadline, ok)
				}
				if r.URL.Host == "device.invalid" && r.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Error("device host lost scoped authentication")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header), Request: r}, nil
			})
			var stdout, stderr bytes.Buffer
			args := append([]string{"--config", filepath.Join(t.TempDir(), "config.yaml"), "--timeout", "50ms", "--trace-http"}, tc.args...)
			code := Execute(context.Background(), args, strings.NewReader(""), &stdout, &stderr)
			if tc.args[0] != "auth" && code != exitOK {
				t.Errorf("code=%d stderr=%s", code, &stderr)
			}
			if !strings.Contains(stderr.String(), "[http]") {
				t.Error("HTTP trace missing")
			}
		})
	}
}

func TestProviderErrorSecrets(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	t.Setenv("GOVEETL_BASE_URL", "http://localhost")
	t.Setenv("GOVEETL_DEVICE_BASE_URL", "http://device.invalid")
	t.Setenv("GOVEETL_CLIENT_ID", "fixture-client")
	t.Setenv("GOVEETL_TOKEN", "fixture-token")
	t.Setenv("GOVEETL_PASSWORD", "fixture-password")
	t.Setenv("GOVEETL_VERIFICATION_CODE", "fixture-code")
	http.DefaultTransport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/app/v1/account/iot/key" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":200,"data":{"endpoint":"fixture"}}`)), Header: make(http.Header), Request: r}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":401,"message":"fixture-token fixture-password fixture-code"}`)), Header: make(http.Header), Request: r}, nil
	})
	for _, args := range [][]string{{"devices", "list", "--backend", "app"}, {"auth", "login", "--email", "fixture@example.invalid", "--no-input"}, {"mqtt", "topic", "--device", "test", "--sku", "H6006"}} {
		var stdout, stderr bytes.Buffer
		code := Execute(context.Background(), append([]string{"--config", filepath.Join(t.TempDir(), "config.yaml")}, args...), strings.NewReader(""), &stdout, &stderr)
		if code != exitErr || stdout.Len() != 0 {
			t.Errorf("code=%d stdout=%s", code, &stdout)
		}
		secrets := []string{"fixture-token"}
		if args[0] == "auth" {
			secrets = append(secrets, "fixture-password", "fixture-code")
		}
		for _, secret := range secrets {
			if strings.Contains(stderr.String(), secret) {
				t.Errorf("error exposes %s", secret)
			}
		}
	}
}

func TestRawJSONInputForms(t *testing.T) {
	const exact = `{ "id":9007199254740993, "id":1e100 }`
	for _, tc := range []struct {
		name, input   string
		file, nonJSON bool
	}{
		{"data", exact, false, false}, {"file", exact, true, false},
		{"null", "null", false, false}, {"non-JSON-response", exact, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := httpFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != tc.input {
					t.Errorf("body=%q error=%v", body, err)
				}
				response := tc.input
				if tc.nonJSON {
					response = "not JSON"
				}
				_, _ = io.WriteString(w, response)
			}))
			args := []string{"--config", filepath.Join(t.TempDir(), "config.yaml"), "--base-url", baseURL, "raw", "POST", "/", "--json"}
			if tc.file {
				path := filepath.Join(t.TempDir(), "body.json")
				if err := os.WriteFile(path, []byte(tc.input), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--file", path)
			} else {
				args = append(args, "--data", tc.input)
			}
			var stdout, stderr bytes.Buffer
			code := Execute(context.Background(), args, strings.NewReader(""), &stdout, &stderr)
			if tc.nonJSON {
				if code != exitErr || stdout.Len() != 0 || stderr.Len() == 0 {
					t.Errorf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
				}
			} else if code != exitOK || stdout.String() != tc.input+"\n" || stderr.Len() != 0 {
				t.Errorf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
			}
		})
	}
}

func TestCanceledLANCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, args := range [][]string{{"lan", "status", "127.0.0.1"}, {"lan", "discover", "--address", "127.0.0.1"}} {
		var stdout, stderr bytes.Buffer
		code := Execute(ctx, append([]string{"--config", filepath.Join(t.TempDir(), "config.yaml")}, args...), strings.NewReader(""), &stdout, &stderr)
		if code != exitErr || stdout.Len() != 0 || !strings.Contains(stderr.String(), "context canceled") {
			t.Errorf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
		}
	}
}

func TestVersionJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--json", "version"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"version": "dev"`) {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--version"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "version dev") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestDoctor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	t.Setenv(config.EnvPrefix+"_BASE_URL", server.URL)

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--config", filepath.Join(t.TempDir(), "config.yaml"), "--json", "doctor"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	// doctor reports every backend; nothing is configured here
	out := stdout.String()
	for _, key := range []string{`"app": {`, `"openapi": {`, `"token": false`} {
		if !strings.Contains(out, key) {
			t.Fatalf("stdout = %s", out)
		}
	}
}

func TestRawUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"raw", "GET"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
}

func TestDryRunPreventsSideEffects(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	for _, key := range []string{"BASE_URL", "OPENAPI_BASE_URL", "DEVICE_BASE_URL"} {
		t.Setenv("GOVEETL_"+key, server.URL)
	}
	t.Setenv("GOVEETL_API_KEY", "test-key")
	t.Setenv("GOVEETL_TOKEN", "test-token")
	t.Setenv("GOVEETL_REFRESH_TOKEN", "test-refresh")
	t.Setenv("GOVEETL_PASSWORD", "test-password")
	for _, args := range [][]string{
		{"control", "test:H6006", "brightness", "65"},
		{"control", "test:H6006", "turn", "1", "--backend", "mqtt"},
		{"groups", "control", "42", "1"},
		{"lan", "control", "127.0.0.1", "turn", `{"value":1}`},
		{"auth", "login", "--no-input"},
		{"auth", "refresh"},
		{"auth", "logout"},
		{"config", "set", "email", "changed@example.com"},
		{"config", "init", "--force"},
		{"raw", "POST", "/mutate", "--data", "{}"},
		{"raw", "post", "/mutate", "--backend", "app", "--data", "{}"},
		{"automations", "set", "42", "test:H706C", "--brightness", "20"},
		{"automations", "remove-device", "42", "test:H616C"},
	} {
		t.Run(strings.Join(args[:2], " ")+strings.Join(args[2:], " "), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			before := []byte("email: original@example.com\n")
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			requests.Store(0)
			var stdout, stderr bytes.Buffer
			code := Execute(context.Background(), append([]string{"--config", path, "--dry-run"}, args...), strings.NewReader(""), &stdout, &stderr)
			if code != exitErr || !strings.Contains(stderr.String(), "dry-run:") {
				t.Errorf("expected dry-run refusal, code=%d stderr=%s", code, &stderr)
			}
			if requests.Load() != 0 {
				t.Errorf("sent %d requests", requests.Load())
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Errorf("config changed: %v", err)
			}
		})
	}
}

func TestSceneCatalogs(t *testing.T) {
	for _, diy := range []bool{false, true} {
		t.Run(fmt.Sprint(diy), func(t *testing.T) {
			endpoint := "/router/api/v1/device/scenes"
			if diy {
				endpoint = "/router/api/v1/device/diy-scenes"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Payload struct{ Device, Sku string } }
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.Method != "POST" || r.URL.Path != endpoint || body.Payload.Device != "AA:BB" || body.Payload.Sku != "H706C" {
					t.Errorf("request: %s %s %+v", r.Method, r.URL.Path, body)
				}
				_, _ = w.Write([]byte(`{"code":200,"msg":"success","payload":{"device":"AA:BB","sku":"H706C","capabilities":[{"type":"devices.capabilities.dynamic_scene","instance":"lightScene","parameters":{"options":[{"name":"Candlelight","value":{"id":123,"paramId":456}}]}}]}}`))
			}))
			defer server.Close()
			t.Setenv("GOVEETL_OPENAPI_BASE_URL", server.URL)
			t.Setenv("GOVEETL_API_KEY", "test-key")
			args := []string{"--config", filepath.Join(t.TempDir(), "config.yaml"), "--dry-run", "--json", "scenes", "AA:BB:H706C"}
			if diy {
				args = append(args, "--diy")
			}
			var stdout, stderr bytes.Buffer
			if code := Execute(context.Background(), args, strings.NewReader(""), &stdout, &stderr); code != exitOK {
				t.Fatalf("code=%d stderr=%s", code, &stderr)
			}
			if !strings.Contains(stdout.String(), `"Candlelight"`) || !strings.Contains(stdout.String(), `"paramId": 456`) {
				t.Fatalf("scene catalog lost: %s", &stdout)
			}
		})
	}
}

func TestMQTTControlBroker(t *testing.T) {
	tlsFixture := httptest.NewTLSServer(nil)
	cert := tlsFixture.TLS.Certificates[0]
	tlsFixture.Close()
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprint(reject), func(t *testing.T) {
			broker, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer broker.Close()
			published := make(chan *packets.PublishPacket, 1)
			go func() {
				conn, err := broker.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				if _, err := packets.ReadPacket(conn); err != nil {
					return
				}
				ack := packets.NewControlPacket(packets.Connack).(*packets.ConnackPacket)
				if reject {
					ack.ReturnCode = 5
				}
				if err := ack.Write(conn); err != nil || reject {
					return
				}
				packet, err := packets.ReadPacket(conn)
				if err != nil {
					return
				}
				if pub, ok := packet.(*packets.PublishPacket); ok {
					published <- pub
				}
				_, _ = packets.ReadPacket(conn)
			}()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/app/v1/account/iot/key":
					_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "data": map[string]string{
						"endpoint":       "tcp://" + broker.Addr().String(),
						"certificatePem": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})),
						"privateKey":     string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})),
					}})
				case "/device/rest/devices/v1/appDeviceTopic":
					_, _ = w.Write([]byte(`{"status":200,"topic":"device/commands"}`))
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			t.Setenv("GOVEETL_BASE_URL", server.URL)
			t.Setenv("GOVEETL_DEVICE_BASE_URL", server.URL)
			t.Setenv("GOVEETL_TOKEN", "test-token")
			t.Setenv("GOVEETL_CLIENT_ID", "test-client")
			t.Setenv("GOVEETL_ACCOUNT_ID", "42")
			t.Setenv("GOVEETL_ACCOUNT_TOPIC", "account/replies")
			var stdout, stderr bytes.Buffer
			args := []string{"--config", filepath.Join(t.TempDir(), "config.yaml"), "control", "AA:BB:H706C", "turn", `{"val":1}`, "--backend", "mqtt"}
			code := Execute(context.Background(), args, strings.NewReader(""), &stdout, &stderr)
			if reject {
				if code != exitErr || !strings.Contains(stderr.String(), "mqtt: connect:") {
					t.Fatalf("code=%d stderr=%s", code, &stderr)
				}
				return
			}
			if code != exitOK {
				t.Fatalf("code=%d stderr=%s", code, &stderr)
			}
			select {
			case pub := <-published:
				var envelope struct {
					Msg map[string]any `json:"msg"`
				}
				if err := json.Unmarshal(pub.Payload, &envelope); err != nil {
					t.Fatal(err)
				}
				if pub.TopicName != "device/commands" || envelope.Msg["accountTopic"] != "account/replies" || envelope.Msg["cmd"] != "turn" {
					t.Fatalf("topic=%s envelope=%v", pub.TopicName, envelope)
				}
			case <-time.After(time.Second):
				t.Fatal("no MQTT command received")
			}
		})
	}
}
