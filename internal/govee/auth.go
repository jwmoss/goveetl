package govee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/jwmoss/goveetl/internal/api"
)

var ErrVerificationRequired = errors.New("email verification required")

// Login uses the account REST endpoint used by Homebridge, not the encrypted bff login.
func (a *App) Login(ctx context.Context, email, password, code string) (session *LoginData, err error) {
	defer func() { err = a.client.RedactError(err, password, code) }()
	body := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Client   string `json:"client"`
		Code     string `json:"code,omitempty"`
	}{email, password, a.headers.Get("clientId"), code}
	data, err := a.accountRequest(ctx, "/account/rest/account/v2/login", body)
	if err != nil {
		return nil, err
	}
	var response struct {
		Status  int       `json:"status"`
		Message string    `json:"message"`
		Client  LoginData `json:"client"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	if response.Status == 454 {
		return nil, ErrVerificationRequired
	}
	if response.Status != 200 {
		return nil, &Error{Status: response.Status, Message: response.Message}
	}
	if strings.TrimSpace(response.Client.Token) == "" || response.Client.AccountID <= 0 || strings.TrimSpace(response.Client.Topic) == "" {
		return nil, fmt.Errorf("Govee returned an incomplete app session")
	}
	return &response.Client, nil
}

func (a *App) RequestVerification(ctx context.Context, email string) (err error) {
	defer func() { err = a.client.RedactError(err) }()
	body := struct {
		Email string `json:"email"`
		Type  int    `json:"type"`
	}{email, 8}
	data, err := a.accountRequest(ctx, "/account/rest/account/v1/verification", body)
	if err != nil {
		return err
	}
	var response Envelope[json.RawMessage]
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	return response.Err()
}

func (a *App) accountRequest(ctx context.Context, path string, body any) ([]byte, error) {
	endpoint, err := url.Parse(a.client.BaseURL())
	if err != nil {
		return nil, err
	}
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && (endpoint.Hostname() == "localhost" || net.ParseIP(endpoint.Hostname()).IsLoopback())) {
		return nil, fmt.Errorf("account authentication requires HTTPS")
	}
	headers := a.headers.Clone()
	headers.Set("appVersion", "7.4.10")
	headers.Set("clientType", "1")
	headers.Set("iotVersion", "0")
	// Login and verification must not send a previous account's bearer token.
	options := append([]api.Option(nil), a.options...)
	options = append(options, api.WithAuth("Authorization", "", ""), api.WithNoRedirects(), api.WithUserAgent("GoveeHome/7.4.10 (com.ihoment.GoVeeSensor; build:8; iOS 26.5.0) Alamofire/5.11.0"))
	client := api.New(a.client.BaseURL(), options...)
	return client.DoWithHeaders(ctx, http.MethodPost, path, nil, body, headers)
}
