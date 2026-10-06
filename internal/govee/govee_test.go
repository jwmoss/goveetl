package govee_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jwmoss/goveetl/internal/api"
	"github.com/jwmoss/goveetl/internal/govee"
)

type fixtureRoundTripper func(*http.Request) (*http.Response, error)

func (f fixtureRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAccountRedirectPolicy(t *testing.T) {
	requests, callbacks := 0, 0
	supplied := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { callbacks++; return nil }, Transport: fixtureRoundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Header.Get("Authorization") != "" {
			t.Error("account request sends the previous token")
		}
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": {"/credential-sink"}}, Body: io.NopCloser(strings.NewReader("{}")), Request: r}, nil
	})}
	app := govee.NewApp("http://localhost", "fixture-token", "7.6.21", govee.AppHeaders{}, api.WithHTTPClient(supplied))
	if _, err := app.Login(context.Background(), "fixture@example.invalid", "fixture-password", ""); err == nil {
		t.Error("redirect login succeeded")
	}
	if err := app.RequestVerification(context.Background(), "fixture@example.invalid"); err == nil {
		t.Error("redirect verification succeeded")
	}
	if requests != 2 || callbacks != 0 || supplied.CheckRedirect == nil || supplied.Timeout != 0 {
		t.Fatalf("requests=%d callbacks=%d timeout=%s", requests, callbacks, supplied.Timeout)
	}
}

func TestDeviceIOCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sender := &govee.MqttSender{}
	if err := sender.ConnectContext(ctx, "fixture.invalid", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("connect error=%v", err)
	}
	if err := sender.SendRawContext(ctx, "fixture-topic", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("publish error=%v", err)
	}
	if _, err := govee.NewLAN().ControlContext(ctx, "127.0.0.1", govee.LANMessage{Cmd: "turn"}, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("LAN error=%v", err)
	}
}

func TestEnvelopeOKAndErr(t *testing.T) {
	var env govee.Envelope[json.RawMessage]
	if err := json.Unmarshal([]byte(`{"status":200,"message":"Success","data":{"a":1}}`), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK() {
		t.Fatal("expected ok envelope")
	}
	if err := json.Unmarshal([]byte(`{"status":1010,"message":"bad token"}`), &env); err != nil {
		t.Fatal(err)
	}
	if env.OK() {
		t.Fatal("expected failure envelope")
	}
	err := env.Err()
	if err == nil || !strings.Contains(err.Error(), "1010") {
		t.Fatalf("error = %v", err)
	}
}

func TestPatchPad(t *testing.T) {
	if got := govee.PatchPad("7.6.21"); got != "7.6.21" {
		t.Fatalf("7.6.21 -> %q", got)
	}
	if got := govee.PatchPad("1.2.3"); got != "1.2.30" {
		t.Fatalf("1.2.3 -> %q", got)
	}
}

func TestAppHeadersAttachedOnRealRequest(t *testing.T) {
	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte(`{"status":200,"message":"Success","data":{"devices":[]}}`))
	}))
	defer server.Close()

	app := govee.NewApp(server.URL, "token-1", "1.2.3", govee.AppHeaders{
		AppVersion: "1.2.3", ClientID: "uuid-1", SysVersion: "31", IotVersion: "6",
		Language: "en-US", Country: "US", TimeZone: "UTC",
	})
	if _, err := app.DeviceList(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := got.Get("appVersion"); v != "1.2.30" {
		t.Fatalf("appVersion = %q", v)
	}
	if v := got.Get("clientType"); v != "0" {
		t.Fatalf("clientType = %q", v)
	}
	if v := got.Get("Accept-Language"); v != "en-US" {
		t.Fatalf("accept-language = %q", v)
	}
	if v := got.Get("Authorization"); v != "Bearer token-1" {
		t.Fatalf("authorization = %q", v)
	}
}

func TestLANPlaintextStatus(t *testing.T) {
	device, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4003})
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	packets := make(chan string, 1)
	go func() {
		_ = device.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 4096)
		n, addr, err := device.ReadFromUDP(buf)
		if err != nil {
			packets <- err.Error()
			return
		}
		packets <- string(buf[:n])
		_, _ = device.WriteToUDP([]byte(`{"msg":{"cmd":"scan","data":{}}}`), &net.UDPAddr{IP: addr.IP, Port: 4002})
		_, _ = device.WriteToUDP([]byte(`{"msg":{"cmd":"devStatus","data":{"brightness":60}}}`), &net.UDPAddr{IP: addr.IP, Port: 4002})
	}()
	reply, err := govee.NewLAN().Control("127.0.0.1", govee.LANMessage{Cmd: "devStatus", Data: map[string]any{}}, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got := <-packets; got != `{"msg":{"cmd":"devStatus","data":{}}}` {
		t.Fatalf("packet = %s", got)
	}
	if string(reply) != `{"msg":{"cmd":"devStatus","data":{"brightness":60}}}` {
		t.Fatalf("reply = %s", reply)
	}
	if _, err := govee.NewLAN().Control("127.0.0.1", govee.LANMessage{Cmd: "devStatus"}, 20*time.Millisecond); err == nil {
		t.Fatal("missing status reply must fail")
	}
}

func TestSameModeGroupDevices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bff-app/v1/general-control/list" {
			_, _ = w.Write([]byte(`{"status":200,"data":{"list":[{"groupId":42,"type":4,"devices":[{"device":"AA:BB","sku":"H6006","deviceName":"Porch"}]}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":200,"data":[]}`))
	}))
	defer server.Close()
	app := govee.NewApp(server.URL, "test-token", "7.6.21", govee.AppHeaders{})
	devices, err := app.GroupDevices(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if len(*devices) != 1 || (*devices)[0].Device != "AA:BB" {
		t.Fatalf("devices=%v", devices)
	}
}

func TestWriteEnvelopeShape(t *testing.T) {
	envelope, err := govee.WriteEnvelope("tx-1", "aws/smarthome/users/42", "turn", 1, map[string]int{"val": 1})
	if err != nil {
		t.Fatal(err)
	}
	message, ok := envelope["msg"].(map[string]any)
	if !ok {
		t.Fatalf("msg envelope missing: %v", envelope)
	}
	if message["type"] != 1 || message["origin"] != 32 {
		t.Fatalf("envelope = %#v", envelope)
	}
	if message["accountTopic"] != "aws/smarthome/users/42" {
		t.Fatalf("accountTopic = %v", message["accountTopic"])
	}
	if _, err := govee.WriteEnvelope("tx-1", "", "turn", 1, nil); err == nil {
		t.Fatal("expected error for empty account topic")
	}
}

func TestOpenAPIControlRequestShape(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Govee-API-Key"); got != "key-1" {
			t.Fatalf("api key header = %q", got)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		_, _ = w.Write([]byte(`{"code":200,"message":"Success"}`))
	}))
	defer server.Close()

	openAPI := govee.NewOpenAPI(server.URL, "key-1")
	if err := openAPI.Control(context.Background(), "H6123:12A3", "H6123", "turn", "control.turn", 1); err != nil {
		t.Fatal(err)
	}
	if captured["payload"] == nil {
		t.Fatal("payload missing")
	}
	if captured["requestId"] == "" {
		t.Fatal("requestId missing")
	}
}
