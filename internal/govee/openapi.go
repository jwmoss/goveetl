package govee

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/jwmoss/goveetl/internal/api"
)

// OpenAPI talks to the official public API (developer.govee.com "v2 router"
// contract) at https://openapi.api.govee.com with the Govee-API-Key header.
//
// Endpoints (per the official reference, fetched 2026-10):
//
//	GET  /router/api/v1/user/devices   — devices + capabilities (30/min)
//	POST /router/api/v1/device/state   — {requestId, payload{sku,device}} (30/min/device)
//	POST /router/api/v1/device/control — {requestId, payload{sku,device,capability{type,instance,value}}}
//	POST /router/api/v1/device/scenes  — dynamic scene set
type OpenAPI struct {
	client *api.Client
}

// NewOpenAPI targets the official endpoint with a Govee API key.
func NewOpenAPI(baseURL, apiKey string) *OpenAPI {
	return &OpenAPI{client: api.New(baseURL,
		api.WithAuth("Govee-API-Key", "", apiKey),
		api.WithTimeout(30*time.Second),
		api.WithUserAgent("goveetl/1"),
	)}
}

// OpenAPICapability is one capability entry as returned by the device list
// (also carries `state` objects on state responses).
type OpenAPICapability struct {
	Type       string           `json:"type"`
	Instance   string           `json:"instance"`
	Parameters json.RawMessage  `json:"parameters,omitempty"`
	State      *OpenAPICapState `json:"state,omitempty"`
}

// OpenAPICapState is the reported value of one capability instance.
type OpenAPICapState struct {
	Value any  `json:"value"`
	Error *any `json:"error,omitempty"`
}

// OpenAPIDevice is one entry of the device list.
type OpenAPIDevice struct {
	Sku          string              `json:"sku"`
	Device       string              `json:"device"`
	DeviceName   string              `json:"deviceName,omitempty"`
	Type         string              `json:"type,omitempty"`
	Capabilities []OpenAPICapability `json:"capabilities,omitempty"`
}

// OpenAPIDeviceListResult is the list response envelope.
type OpenAPIDeviceListResult struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    []OpenAPIDevice `json:"data"`
}

// OpenAPIStateResult is the state response envelope. The state endpoint
// returns the device object under payload and its message key under msg.
type OpenAPIStateResult struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Msg     string           `json:"msg"`
	Payload *OpenAPIDevice   `json:"payload"`
	Data    []*OpenAPIDevice `json:"data"`
}

// DeviceData takes the device object from whichever field the server used.
func (r OpenAPIStateResult) DeviceData() []*OpenAPIDevice {
	if r.Payload != nil {
		return []*OpenAPIDevice{r.Payload}
	}
	return r.Data
}

// Text takes the message in either spelling.
func (r OpenAPIStateResult) Text() string {
	if r.Msg != "" {
		return r.Msg
	}
	return r.Message
}

// OpenAPISceneResult is the dynamic-scene list returned for one device.
type OpenAPISceneResult struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// requestID matches the uuid-shaped requestId the API expects.
func requestID() string { return uuid.NewString() }

func (o *OpenAPI) unwrapCode(code int, message string) error {
	if code == 0 || code == 200 {
		return nil
	}
	return &Error{Status: code, Message: message}
}

// ListDevices returns all devices with their capabilities.
func (o *OpenAPI) ListDevices(ctx context.Context) (*OpenAPIDeviceListResult, error) {
	var out OpenAPIDeviceListResult
	if err := o.client.DoJSON(ctx, http.MethodGet, "/router/api/v1/user/devices", nil, nil, &out); err != nil {
		return nil, err
	}
	if err := o.unwrapCode(out.Code, out.Message); err != nil {
		return nil, err
	}
	return &out, nil
}

// State reads the state of one device.
func (o *OpenAPI) State(ctx context.Context, device, sku string) (*OpenAPIStateResult, error) {
	body := map[string]any{
		"requestId": requestID(),
		"payload":   map[string]string{"device": device, "sku": sku},
	}
	var out OpenAPIStateResult
	if err := o.client.DoJSON(ctx, http.MethodPost, "/router/api/v1/device/state", nil, body, &out); err != nil {
		return nil, err
	}
	if err := o.unwrapCode(out.Code, out.Text()); err != nil {
		return nil, err
	}
	return &out, nil
}

// CapabilityType instances per the official reference.
const (
	CapOnOff        = "devices.capabilities.on_off"
	CapToggle       = "devices.capabilities.toggle"
	CapRange        = "devices.capabilities.range"
	CapMode         = "devices.capabilities.mode"
	CapColorSetting = "devices.capabilities.color_setting"
	CapSegment      = "devices.capabilities.segment_color_setting"
	CapMusic        = "devices.capabilities.music_setting"
	CapDynamicScene = "devices.capabilities.dynamic_scene"
	CapWorkMode     = "device.capabilities.work_mode"
	CapTempSetting  = "device.capabilities.temperature_setting"
)

// capabilityRoute maps short command names to (capability type, instance).
var capabilityRoute = map[string]struct{ Type, Instance string }{
	"turn":              {CapOnOff, "powerSwitch"},
	"power":             {CapOnOff, "powerSwitch"},
	"powerSwitch":       {CapOnOff, "powerSwitch"},
	"brightness":        {CapRange, "brightness"},
	"humidity":          {CapRange, "humidity"},
	"volume":            {CapRange, "volume"},
	"temperature":       {CapRange, "temperature"},
	"color":             {CapColorSetting, "colorRgb"},
	"colorTemp":         {CapColorSetting, "colorTemperatureK"},
	"colorTemperatureK": {CapColorSetting, "colorTemperatureK"},
	"segmentColorRgb":   {CapSegment, "segmentedColorRgb"},
	"segmentBrightness": {CapSegment, "segmentedBrightness"},
	"musicMode":         {CapMusic, "musicMode"},
	"lightScene":        {CapDynamicScene, "lightScene"},
	"diyScene":          {CapDynamicScene, "diyScene"},
	"snapshot":          {CapDynamicScene, "snapshot"},
	"gearMode":          {CapMode, "gearMode"},
	"fanSpeed":          {CapMode, "fanSpeed"},
	"oscillationToggle": {CapToggle, "oscillationToggle"},
	"nightlightToggle":  {CapToggle, "nightlightToggle"},
}

// CapRoute resolves a short command name ("turn", "color", ...) or an
// explicit instance name ("powerSwitch") to (capability type, instance).
func CapRoute(name string) (capType, instance string, ok bool) {
	if route, hit := capabilityRoute[name]; hit {
		return route.Type, route.Instance, true
	}
	// certain full form: "<type>/<instance>"
	if type_, inst, chose := cut2(name, "/"); chose && capabilityHasType(type_) {
		return type_, inst, true
	}
	// exact instance names route through their documented type
	for _, route := range capabilityRoute {
		if route.Instance == name {
			return route.Type, route.Instance, true
		}
	}
	return "", "", false
}

func capabilityHasType(capType string) bool {
	for _, route := range capabilityRoute {
		if route.Type == capType {
			return true
		}
	}
	return false
}

func cut2(s, sep string) (before, after string, found bool) {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return s[:i], s[i+len(sep):], true
		}
	}
	return s, "", false
}

// Control sends one capability value to the device with an explicit
// capability type and instance (use CapRoute to resolve short names).
func (o *OpenAPI) Control(ctx context.Context, device, sku, capType, instance string, value any) error {
	payload := map[string]any{
		"sku":    sku,
		"device": device,
		"capability": map[string]any{
			"type":     capType,
			"instance": instance,
			"value":    value,
		},
	}
	var out struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	err := o.client.DoJSON(ctx, http.MethodPost, "/router/api/v1/device/control", nil,
		map[string]any{"requestId": requestID(), "payload": payload}, &out)
	if err != nil {
		return err
	}
	return o.unwrapCode(out.Code, out.Message)
}

// Scenes lists the dynamic scenes (light scenes) of one device.
func (o *OpenAPI) Scenes(ctx context.Context, device, sku string) (*OpenAPISceneResult, error) {
	body := map[string]any{
		"requestId": requestID(),
		"payload":   map[string]string{"device": device, "sku": sku},
	}
	var out OpenAPISceneResult
	if err := o.client.DoJSON(ctx, http.MethodPost, "/router/api/v1/device/scenes", nil, body, &out); err != nil {
		return nil, err
	}
	if err := o.unwrapCode(out.Code, out.Message); err != nil {
		return nil, err
	}
	return &out, nil
}
