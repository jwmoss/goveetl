package govee_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jwmoss/goveetl/internal/govee"
)

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

func TestLANAESEncryptDecryptRoundtrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 16)
	plaintext := govee.LANMessage{Cmd: "on", Data: map[string]int{"val": 1}}
	ciphertext, err := govee.EncryptLANMessage(key, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if len(ciphertext)%16 != 0 || len(ciphertext) == 0 {
		t.Fatalf("ciphertext length %d", len(ciphertext))
	}
	data, err := govee.DecryptLANMessage(key, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	var back govee.LANMessage
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Cmd != "on" {
		t.Fatalf("cmd = %q", back.Cmd)
	}
}

func TestLANRejectsShortKey(t *testing.T) {
	if _, err := govee.EncryptLANMessage(bytes.Repeat([]byte{1}, 8), govee.LANMessage{Cmd: "on"}); err == nil {
		t.Fatal("expected error for 8-byte key")
	}
}

func TestWriteEnvelopeShape(t *testing.T) {
	envelope, err := govee.WriteEnvelope("tx-1", "aws/smarthome/users/42", "turn", 1, map[string]int{"val": 1})
	if err != nil {
		t.Fatal(err)
	}
	if envelope["type"] != 1 || envelope["origin"] != 32 {
		t.Fatalf("envelope = %#v", envelope)
	}
	if envelope["accountTopic"] != "aws/smarthome/users/42" {
		t.Fatalf("accountTopic = %v", envelope["accountTopic"])
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
