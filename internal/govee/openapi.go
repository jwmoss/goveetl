package govee

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/jwmoss/goveetl/internal/api"
)

// OpenAPI talks to the official public API at openapi.api.govee.com
// authenticated by the Govee-API-Key header.
//
// ponytail: capability payloads are pass-through maps; type them once the
// official examples cover the command you need.
type OpenAPI struct {
	client *api.Client
}

// NewOpenAPI targets the official endpoint with a Govee API key.
func NewOpenAPI(baseURL, apiKey string) *OpenAPI {
	return &OpenAPI{client: api.New(baseURL, api.WithAuth("Govee-API-Key", "", apiKey), api.WithTimeout(30*time.Second))}
}

// OpenAPIDevice is one entry of POST /v1/user/devices.
type OpenAPIDevice struct {
	Device           string          `json:"device"`
	DeviceName       string          `json:"deviceName"`
	Sku              string          `json:"sku"`
	DefaultColorTemp json.RawMessage `json:"defaultColorTemp,omitempty"`
	Extension        struct {
		DeviceName      string `json:"deviceName,omitempty"`
		Controllable    bool   `json:"controllable,omitempty"`
		Retrievable     bool   `json:"retrievable,omitempty"`
		Http            bool   `json:"supportHttp,omitempty"`
		TypeCode        int    `json:"typeCode,omitempty"`
		HardwareVersion string `json:"hardwareVersion,omitempty"`
		FirmwareVersion string `json:"firmwareVersion,omitempty"`
		LastConnectTime int64  `json:"lastConnectTime,omitempty"`
	} `json:"extension,omitempty"`
}

// OpenAPIDeviceList is the POST /v1/user/devices response.
type OpenAPIDeviceList struct {
	Data      []OpenAPIDevice `json:"data"`
	Code      int             `json:"code"`
	RetStatus int             `json:"retStatus"`
	RetCode   int             `json:"retCode"`
	Message   string          `json:"message"`
}

// OpenAPICapabilitiesResponse is GET /v1/device/capabilities.
type OpenAPICapabilitiesResponse struct {
	Data []OpenAPICapabilityGroup `json:"data"`
}

// OpenAPICapabilityGroup lists capabilities for one device.
type OpenAPICapabilityGroup struct {
	Device       string              `json:"device"`
	Sku          string              `json:"sku"`
	Capabilities []OpenAPICapability `json:"capabilities"`
}

// OpenAPICapability identifies one device skill.
type OpenAPICapability struct {
	Type     string          `json:"type"`
	Instance json.RawMessage `json:"instance,omitempty"`
}

// OpenAPIState is POST /v1/device/state payload.
type OpenAPIState struct {
	Device string                   `json:"device"`
	Sku    string                   `json:"sku"`
	State  []OpenAPIStateCapability `json:"state"`
}

// OpenAPIStateCapability is one state attribute.
type OpenAPIStateCapability struct {
	Instance string `json:"instance"`
	State    any    `json:"state"`
}

// requestID matches the uuid-shaped requestId the API expects.
func requestID() string { return uuid.NewString() }

// ListDevices returns all devices on the account.
func (o *OpenAPI) ListDevices(ctx context.Context) (*OpenAPIDeviceList, error) {
	var out OpenAPIDeviceList
	err := o.client.DoJSON(ctx, http.MethodPost, "/v1/user/devices", nil,
		map[string]any{"requestId": requestID()}, &out)
	if err != nil {
		return nil, err
	}
	if err := checkOpenAPI(out.Code, out.RetStatus, out.RetCode, out.Message); err != nil {
		return nil, err
	}
	return &out, nil
}

// State reads the current state of one device.
func (o *OpenAPI) State(ctx context.Context, device, sku string) (*OpenAPIState, error) {
	panicless := map[string]any{
		"requestId": requestID(),
		"payload":   map[string]string{"device": device, "sku": sku},
	}
	var out OpenAPIState
	if err := o.client.DoJSON(ctx, http.MethodPost, "/v1/device/state", nil, panicless, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Capabilities lists commands a device supports.
func (o *OpenAPI) Capabilities(ctx context.Context, device, sku string) (*OpenAPICapabilitiesResponse, error) {
	q := map[string][]string{
		"capabilityType": {"v2"},
		"sku":            {sku},
		"device":         {device},
	}
	var out OpenAPICapabilitiesResponse
	if err := o.client.DoJSON(ctx, http.MethodGet, "/v1/device/capabilities", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Control sends a turn/brightness/color/… command. value may be a number,
// string, object or array (e.g. segment colors).
func (o *OpenAPI) Control(ctx context.Context, device, sku, command, instance string, value any) error {
	payload := map[string]any{
		"device": device,
		"sku":    sku,
		"capability": map[string]any{
			"type":  map[string]any{"name": command, "instance": instance},
			"state": value,
		},
	}
	var generic map[string]any
	err := o.client.DoJSON(ctx, http.MethodPost, "/v1/device/state/control", nil,
		map[string]any{"requestId": requestID(), "payload": payload}, &generic)
	if err != nil {
		return err
	}
	return checkOpenAPIMap(generic)
}

// checkOpenAPI validates the official envelope {"code","message","data"}.
func checkOpenAPI(code, retStatus, retCode int, message string) error {
	for _, m := range []struct {
		label string
		value int
	}{
		{"code", code}, {"retStatus", retStatus}, {"retCode", retCode},
	} {
		if m.value != 0 && m.value != 200 {
			return &Error{Status: m.value, Message: m.label + ": " + message}
		}
	}
	return nil
}

func checkOpenAPIMap(generic map[string]any) error {
	code, ok := generic["code"].(float64)
	if !ok || code == 0 || code == 200 {
		return nil
	}
	msg, _ := generic["message"].(string)
	return &Error{Status: int(code), Message: msg}
}
