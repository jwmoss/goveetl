package govee

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jwmoss/goveetl/internal/api"
)

// App talks to the private REST API the Govee Home app uses (app2.govee.com
// /bff-app/... surface) with a captured Bearer token.
type App struct {
	client  *api.Client
	options []api.Option
	// Headers mirrors AppHeader.getAppHeaders() from the app.
	headers http.Header
}

// AppHeaders carries the fixed device-identity headers the app sends on
// every request.
type AppHeaders struct {
	AppVersion string // e.g. 7.6.21
	ClientID   string // stable device UUID
	SysVersion string // OS release
	IotVersion string
	Language   string // e.g. en-US
	Country    string // e.g. US
	TimeZone   string // e.g. America/New_York
}

// NewApp builds the app API client against the given base URL.
func NewApp(baseURL, token, version string, h AppHeaders, opts ...api.Option) *App {
	headers := http.Header{}
	headers.Set("timestamp", fmt.Sprint(time.Now().UnixMilli()))
	headers.Set("country", h.Country)
	// Evidence says "Accept-Language" (decompiled Guava constant). Some
	// services also read "language"; send both until live-verified.
	headers.Set("Accept-Language", h.Language)
	headers.Set("language", h.Language)
	headers.Set("timezone", h.TimeZone)
	headers.Set("appVersion", appVersion(version))
	headers.Set("clientId", h.ClientID)
	headers.Set("envId", "0") // prod run mode number
	headers.Set("sysVersion", h.SysVersion)
	headers.Set("iotVersion", def(h.IotVersion, "6"))
	headers.Set("clientType", "0")
	options := append([]api.Option{api.WithAuth("Authorization", "Bearer", token)}, opts...)
	return &App{client: api.New(baseURL, options...), headers: headers, options: options}
}

func def(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// appVersion pads a single-digit patch to two chars, matching the app's
// AppHeader.appVersion().
func appVersion(version string) string {
	return PatchPad(version)
}

// PatchPad pads a single-digit patch segment to two chars, matching
// AppHeader.appVersion() in the app.
func PatchPad(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) == 3 && len(parts[2]) == 1 {
		return version + "0"
	}
	return version
}

func (a *App) Do(ctx context.Context, method, path string, query map[string][]string, body any) ([]byte, error) {
	data, err := a.client.DoWithHeaders(ctx, method, path, query, body, a.headers)
	if err != nil {
		return data, err
	}
	var env struct {
		Status  *int   `json:"status"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &env) == nil && env.Status != nil && *env.Status != 200 {
		return data, a.client.RedactError(&Error{Status: *env.Status, Message: env.Message})
	}
	return data, nil
}

// Refresh exchanges a refresh token for a fresh bundle.
func (a *App) Refresh(ctx context.Context, refreshToken string) (session *LoginData, err error) {
	defer func() { err = a.client.RedactError(err, refreshToken) }()
	req := RefreshTokenRequest{RefreshToken: refreshToken}
	data, err := a.Do(ctx, http.MethodPost, "/bff-app/v2/account/refresh-token", nil, req)
	if err != nil {
		return nil, err
	}
	var env Envelope[LoginData]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if !env.OK() {
		return nil, env.Err()
	}
	return &env.Data, nil
}

// DeviceList fetches GET /bff-app/v1/device/list.
func (a *App) DeviceList(ctx context.Context) (*DeviceListResponse, error) {
	data, err := a.Do(ctx, http.MethodGet, "/bff-app/v1/device/list", nil, nil)
	if err != nil {
		return nil, err
	}
	var env Envelope[DeviceListResponse]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if !env.OK() {
		return nil, env.Err()
	}
	return &env.Data, nil
}

// GroupList fetches the room-control list.
func (a *App) GroupList(ctx context.Context) ([]byte, error) {
	q := map[string][]string{"filterEmpty": {"false"}}
	return a.Do(ctx, http.MethodGet, "/bff-app/v1/general-control/list", q, nil)
}

// GroupDevices lists devices of one group.
func (a *App) GroupDevices(ctx context.Context, groupID string) (*[]Device, error) {
	list, err := a.GroupList(ctx)
	if err != nil {
		return nil, err
	}
	var groups Envelope[struct {
		List []struct {
			GroupID int      `json:"groupId"`
			Devices []Device `json:"devices"`
		} `json:"list"`
	}]
	if err := json.Unmarshal(list, &groups); err != nil {
		return nil, err
	}
	if err := groups.Err(); err != nil {
		return nil, err
	}
	for _, group := range groups.Data.List {
		if strconv.Itoa(group.GroupID) == groupID && group.Devices != nil {
			return &group.Devices, nil
		}
	}
	q := map[string][]string{"groupId": {groupID}}
	data, err := a.Do(ctx, http.MethodGet, "/bff-app/v1/general-control/group-devices", q, nil)
	if err != nil {
		return nil, err
	}
	var env Envelope[[]Device]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if !env.OK() {
		return nil, env.Err()
	}
	return &env.Data, nil
}

// GroupControl sends a scene/group REST control command.
func (a *App) GroupControl(ctx context.Context, req GroupControlRequest) error {
	data, err := a.Do(ctx, http.MethodPost, "/bff-app/v1/general-control/iot-control", nil, req)
	if err != nil {
		return err
	}
	var env Envelope[json.RawMessage]
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	return env.Err()
}

// DeviceTopic resolves the per-device MQTT publish topic
// (POST device/rest/devices/v1/appDeviceTopic on the device host).
func (a *App) DeviceTopic(ctx context.Context, sku, device, deviceBaseURL string) (topic string, err error) {
	defer func() { err = a.client.RedactError(err) }()
	body := struct {
		Transaction string `json:"transaction"`
		Sku         string `json:"sku"`
		Device      string `json:"device"`
	}{Transaction: uuid.NewString(), Sku: sku, Device: device}
	if deviceBaseURL != "" && deviceBaseURL != a.client.BaseURL() {
		// The configured device host has its own credential scope.
		client := api.New(deviceBaseURL, a.options...)
		data, httpErr := client.DoWithHeaders(ctx, http.MethodPost, "/device/rest/devices/v1/appDeviceTopic", nil, body, a.headers)
		if httpErr != nil {
			return "", httpErr
		}
		return decodeDeviceTopic(data)
	}
	data, err := a.Do(ctx, http.MethodPost, "/device/rest/devices/v1/appDeviceTopic", nil, body)
	if err != nil {
		return "", err
	}
	return decodeDeviceTopic(data)
}

func decodeDeviceTopic(data []byte) (string, error) {
	var out struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Topic   string `json:"topic"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.Status != 200 {
		return "", &Error{Status: out.Status, Message: out.Message}
	}
	if out.Topic == "" {
		return "", fmt.Errorf("app: device topic missing")
	}
	return out.Topic, nil
}

// IotCert fetches the MQTT mutual-TLS certificate bundle
// (GET app/v1/account/iot/key).
func (a *App) IotCert(ctx context.Context) (*IotCertificate, error) {
	data, err := a.Do(ctx, http.MethodGet, "/app/v1/account/iot/key", nil, nil)
	if err != nil {
		return nil, err
	}
	var env Envelope[IotCertificate]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if !env.OK() {
		return nil, env.Err()
	}
	return &env.Data, nil
}

// ReadAccountTopic asks the REST bridge for the account topic
// (POST account/rest/account/v1/iot).
func (a *App) ReadAccountTopic(ctx context.Context) (*IotAccountTopic, error) {
	data, err := a.Do(ctx, http.MethodPost, "/account/rest/account/v1/iot", nil, map[string]any{"transaction": uuid.NewString()})
	if err != nil {
		return nil, err
	}
	var env Envelope[IotAccountTopic]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if !env.OK() {
		return nil, env.Err()
	}
	return &env.Data, nil
}
