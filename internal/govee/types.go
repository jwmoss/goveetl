// Package govee implements clients for the Govee device ecosystem:
// the official OpenAPI (Govee-API-Key), the private app API used by the
// Govee Home app, its AWS IoT MQTT control channel, and the LAN API.
//
// Endpoint facts come from reverse-engineering Govee Home 7.6.21111? 7.6.21 (code 1119).
package govee

import "encoding/json"

// Envelope is the app API JSON wrapper: {"status":..,"message":..,"data":..}.
// Success is status 200.
type Envelope[T any] struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// OK reports whether the envelope reports success.
func (e Envelope[T]) OK() bool { return e.Status == 200 }

func (e Envelope[T]) Err() error {
	if e.OK() {
		return nil
	}
	return &Error{Status: e.Status, Message: e.Message}
}

// Error is an app/OpenAPI-level failure (2xx-wrapped error or non-2xx HTTP).
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return "govee: request failed"
	}
	if e.Status == 0 {
		return "govee: " + e.Message
	}
	return "govee: status " + itoa(e.Status) + ": " + e.Message
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// Device is one entry of the app device list (AbsDevice wire shape).
type Device struct {
	AttributesID int             `json:"attributesId"`
	CardType     int             `json:"cardType"`
	Device       string          `json:"device"`
	DeviceID     int             `json:"deviceId"`
	DeviceName   string          `json:"deviceName"`
	DeviceExt    json.RawMessage `json:"deviceExt,omitempty"`
	Gas          string          `json:"gas,omitempty"`
	Gid          string          `json:"gid,omitempty"`
	GoodsType    int             `json:"goodsType"`
	GroupID      int             `json:"groupId"`
	PactCode     int             `json:"pactCode"`
	PactType     int             `json:"pactType"`
	Position     int             `json:"position"`
	Share        int             `json:"share"`
	Sku          string          `json:"sku"`
	SkuType      int             `json:"skuType"`
	Spec         string          `json:"spec,omitempty"`
	SubDeviceNum int             `json:"subDeviceNum"`
	VersionHard  string          `json:"versionHard,omitempty"`
	VersionSoft  string          `json:"versionSoft,omitempty"`
}

// Group is a room in the app device list (AbsGroup wire shape).
type Group struct {
	GroupID   int    `json:"groupId"`
	GroupName string `json:"groupName"`
	Position  int    `json:"position"`
}

// DeviceListResponse is GET /bff-app/v1/device/list payload.
type DeviceListResponse struct {
	Devices  []Device    `json:"devices"`
	Groups   []Group     `json:"groups"`
	Sort     []SortEntry `json:"sort,omitempty"`
	SortTime int64       `json:"sortTime"`
}

// SortEntry is the home screen order for one device.
type SortEntry struct {
	Device     string `json:"device"`
	GroupID    int    `json:"groupId"`
	GroupIndex int    `json:"groupIndex"`
	Index      int    `json:"index"`
}

// LoginRequest is POST bff-app/v2/account/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Client   string `json:"client"` // stable device UUID
	Code     string `json:"code,omitempty"`
}

// LoginData is the login payload; token authenticates the app API and
// topic is the MQTT account topic to subscribe.
type LoginData struct {
	AccountID        int    `json:"accountId"`
	Token            string `json:"token"`
	RefreshToken     string `json:"refreshToken"`
	TokenExpireCycle int    `json:"tokenExpireCycle"`
	TokenExpireTime  int64  `json:"tokenExpireTime"`
	Topic            string `json:"topic"`
	Client           string `json:"client,omitempty"`
	Email            string `json:"email,omitempty"`
}

// RefreshTokenRequest is POST bff-app/v2/account/refresh-token.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken"`
	Type         int    `json:"type"`
}

// IotCertificate is GET app/v1/account/iot/key payload for the MQTT channel.
type IotCertificate struct {
	CertificatePem string `json:"certificatePem"`
	Endpoint       string `json:"endpoint"`
	PrivateKey     string `json:"privateKey"`
}

// IotAccountTopic is POST account/rest/account/v1/iot payload.
type IotAccountTopic struct {
	AccountTopic string `json:"accountTopic"`
}

// groupControl payloads

// GroupControlDevice identifies one device in group ops.
type GroupControlDevice struct {
	Device string `json:"device"`
	Sku    string `json:"sku"`
}

// GroupControlRequest is POST bff-app/v1/general-control/iot-control body.
type GroupControlRequest struct {
	GroupID   int                  `json:"groupId"`
	OpType    int                  `json:"opType"`
	SceneID   int                  `json:"sceneId"`
	Devices   []GroupControlDevice `json:"devices,omitempty"`
	ColorHval *int                 `json:"colorHval,omitempty"`
}
