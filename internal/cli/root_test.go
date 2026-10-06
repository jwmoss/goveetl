package cli

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
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
