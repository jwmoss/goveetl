package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutomationCommands(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
		fail bool
	}{
		{"list", []string{"list"}, "Sunset", false},
		{"show", []string{"show", "42"}, "Halloween D", false},
		{"set", []string{"set", "42", "AA:BB:H706C", "--power", "on", "--brightness", "60", "--temperature", "2700"}, `"temperature_k": 2700`, false},
		{"brightness", []string{"set", "42", "AA:BB:H706C", "--brightness", "60"}, `"scene": "Halloween D"`, false},
		{"remove", []string{"remove-device", "42", "CC:DD:H616C"}, "House", false},
		{"rejected", []string{"remove-device", "42", "CC:DD:H616C"}, "status 500", true},
		{"unchanged", []string{"remove-device", "42", "CC:DD:H616C"}, "verification", true},
		{"missing", []string{"remove-device", "42", "unknown:H616C"}, "not in", true},
		{"last-device", []string{"remove-device", "42", "CC:DD:H616C"}, "last device", true},
		{"temperature", []string{"set", "42", "CC:DD:H616C", "--temperature", "2700"}, "supports H706C", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var detail map[string]any
			if err := json.Unmarshal([]byte(`{"groupId":42,"name":"Sunset","enable":1,"cmdType":129,"triggerRule":{"deviceObj":null,"terminalId":null,"rule":{"cmdType":129,"cmdVal":"sunset"}},"schedule":{},"linkage":{"alternate":0,"effTime":{"days":[1,2,3],"timeZone":"America/New_York"},"ruleGroups":[{"iotRuleId":null,"actionType":1,"delayTime":0,"iotRules":[{"cmdGroup":1,"deviceObj":{"device":"AA:BB","sku":"H706C","name":"House","topic":"private-device-topic","settings":"preserve"},"rule":[{"cmdType":0,"cmdVal":"{\"open\":1}","iotMsg":"{\"msg\":{\"accountTopic\":\"private-account-topic\",\"cmdVersion\":0,\"origin\":35,\"type\":1,\"cmd\":\"turn\",\"data\":{\"val\":1}}}"},{"cmdType":1,"cmdVal":"{\"brightness\":80}","iotMsg":"{\"msg\":{\"accountTopic\":\"private-account-topic\",\"cmdVersion\":1,\"origin\":35,\"type\":1,\"cmd\":\"brightness\",\"data\":{\"val\":80}}}"},{"cmdType":3,"cmdVal":"{\"name\":\"Halloween D\"}","iotMsg":"scene-message"}]},{"cmdGroup":1,"deviceObj":{"device":"CC:DD","sku":"H616C","name":"Garage"},"rule":[{"cmdType":1,"cmdVal":"{\"brightness\":20}","iotMsg":"unchanged-other-action"}]}]}]}}`), &detail); err != nil {
				t.Fatal(err)
			}
			if tc.name == "last-device" {
				group := detail["linkage"].(map[string]any)["ruleGroups"].([]any)[0].(map[string]any)
				group["iotRules"] = group["iotRules"].([]any)[1:]
			}
			trigger, _ := json.Marshal(detail["triggerRule"])
			effectiveTime, _ := json.Marshal(detail["linkage"].(map[string]any)["effTime"])
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("appVersion") != "7.6.21" || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing app authentication headers")
				}
				if strings.HasSuffix(r.URL.Path, "/automations") {
					_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "data": map[string]any{"autoExecs": []any{map[string]any{"groupId": 42, "name": "Sunset", "enable": 1, "groupSort": 4}}}})
					return
				}
				if r.URL.Query().Get("groupId") != "42" && r.Method == "GET" {
					t.Error("wrong automation ID")
				}
				if r.Method == "PUT" {
					writes++
					var next map[string]any
					if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
						t.Error(err)
					}
					if schedule, ok := next["schedule"].(map[string]any); ok && len(schedule) == 0 {
						t.Error("inactive empty schedule sent instead of omitted")
					}
					trigger := next["triggerRule"].(map[string]any)
					groups := next["linkage"].(map[string]any)["ruleGroups"].([]any)
					if value, exists := trigger["deviceObj"]; exists && value == nil {
						_, _ = w.Write([]byte(`{"status":500,"message":"unexpected null trigger device"}`))
						return
					}
					if value, exists := groups[0].(map[string]any)["iotRuleId"]; exists && value == nil {
						_, _ = w.Write([]byte(`{"status":500,"message":"unexpected null action group ID"}`))
						return
					}
					if tc.name == "rejected" {
						_, _ = w.Write([]byte(`{"status":500,"message":"service is busy"}`))
						return
					}
					if tc.name != "unchanged" {
						detail = next
						// Detail reads restore nullable metadata even though the app omits it on writes.
						detail["triggerRule"].(map[string]any)["deviceObj"] = nil
						detail["triggerRule"].(map[string]any)["terminalId"] = nil
						detail["linkage"].(map[string]any)["ruleGroups"].([]any)[0].(map[string]any)["iotRuleId"] = nil
					}
					_, _ = w.Write([]byte(`{"status":200,"message":"Success"}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "data": detail})
			}))
			defer server.Close()
			t.Setenv("GOVEETL_BASE_URL", server.URL)
			t.Setenv("GOVEETL_TOKEN", "test-token")
			t.Setenv("GOVEETL_CLIENT_ID", "test-client")
			var out, errOut bytes.Buffer
			args := append([]string{"--config", filepath.Join(t.TempDir(), "config.yaml"), "--json", "automations"}, tc.args...)
			code := Execute(context.Background(), args, strings.NewReader(""), &out, &errOut)
			if tc.fail {
				if code != exitErr || !strings.Contains(errOut.String(), tc.want) {
					t.Fatalf("code=%d stderr=%s", code, &errOut)
				}
				if tc.name != "rejected" && tc.name != "unchanged" && writes != 0 {
					t.Fatal("invalid mutation reached the server")
				}
				return
			}
			if code != exitOK || !strings.Contains(out.String(), tc.want) {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, &out, &errOut)
			}
			if strings.Contains(out.String(), "private-") || strings.Contains(out.String(), "iotMsg") {
				t.Fatalf("private messaging data exposed: %s", &out)
			}
			if tc.name != "set" && tc.name != "remove" && tc.name != "brightness" {
				if writes != 0 {
					t.Fatal("read command wrote automation")
				}
				return
			}
			if writes != 1 {
				t.Fatalf("writes=%d", writes)
			}
			if detail["name"] != "Sunset" || detail["enable"] != float64(1) || detail["groupSort"] != float64(4) {
				t.Fatal("automation identity or enable state changed")
			}
			afterTrigger, _ := json.Marshal(detail["triggerRule"])
			afterEffectiveTime, _ := json.Marshal(detail["linkage"].(map[string]any)["effTime"])
			if !bytes.Equal(trigger, afterTrigger) || !bytes.Equal(effectiveTime, afterEffectiveTime) {
				t.Fatal("trigger or effective time changed")
			}
			groups := detail["linkage"].(map[string]any)["ruleGroups"].([]any)
			rules := groups[0].(map[string]any)["iotRules"].([]any)
			if tc.name == "remove" {
				if len(rules) != 1 || rules[0].(map[string]any)["deviceObj"].(map[string]any)["device"] != "AA:BB" {
					t.Fatal("wrong device removed")
				}
				return
			}
			if len(rules) != 2 || rules[1].(map[string]any)["rule"].([]any)[0].(map[string]any)["iotMsg"] != "unchanged-other-action" {
				t.Fatal("other action changed")
			}
			house := rules[0].(map[string]any)
			if house["deviceObj"].(map[string]any)["settings"] != "preserve" {
				t.Fatal("device metadata lost")
			}
			if len(house["rule"].([]any)) != 3 {
				t.Fatal("power, brightness, or light mode missing")
			}
			if tc.name == "brightness" {
				if house["rule"].([]any)[1].(map[string]any)["iotMsg"] != "scene-message" || !strings.Contains(out.String(), `"brightness": 60`) {
					t.Fatal("brightness edit changed the scene or lost brightness")
				}
				return
			}
			for _, item := range house["rule"].([]any) {
				rule := item.(map[string]any)
				var wire struct {
					Msg struct {
						Cmd        string
						Data       map[string]any
						Origin     int
						CmdVersion int
					}
				}
				if err := json.Unmarshal([]byte(rule["iotMsg"].(string)), &wire); err != nil {
					t.Fatal(err)
				}
				if wire.Msg.Origin != 35 {
					t.Fatal("automation origin changed")
				}
				switch int(rule["cmdType"].(float64)) {
				case 0:
					if wire.Msg.Cmd != "turn" || wire.Msg.Data["val"] != float64(1) {
						t.Fatal("power command mismatch")
					}
				case 1:
					if wire.Msg.Cmd != "brightness" || wire.Msg.Data["val"] != float64(60) || wire.Msg.CmdVersion != 1 {
						t.Fatal("brightness command mismatch")
					}
				case 5:
					if wire.Msg.Cmd != "colorwc" || wire.Msg.Data["colorTemInKelvin"] != float64(2700) {
						t.Fatal("temperature command mismatch")
					}
				default:
					t.Fatal("old scene remains")
				}
			}
		})
	}
}

func TestRawAppHeaders(t *testing.T) {
	for _, status := range []int{200, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("appVersion") != "7.6.21" || r.Header.Get("clientId") != "test-client" {
					t.Error("app headers absent")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "message": "server result", "data": map[string]any{}})
			}))
			defer server.Close()
			t.Setenv("GOVEETL_BASE_URL", server.URL)
			t.Setenv("GOVEETL_TOKEN", "test-token")
			t.Setenv("GOVEETL_CLIENT_ID", "test-client")
			var out, errOut bytes.Buffer
			args := []string{"--config", filepath.Join(t.TempDir(), "config.yaml"), "raw", "GET", "/read", "--backend", "app"}
			code := Execute(context.Background(), args, strings.NewReader(""), &out, &errOut)
			if status == 500 {
				if code != exitErr || !strings.Contains(errOut.String(), "status 500") {
					t.Fatalf("code=%d stderr=%s", code, &errOut)
				}
			} else if code != exitOK {
				t.Fatalf("code=%d stderr=%s", code, &errOut)
			}
		})
	}
}
