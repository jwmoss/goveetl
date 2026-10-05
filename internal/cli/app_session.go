package cli

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jwmoss/goveetl/internal/config"
	"github.com/jwmoss/goveetl/internal/govee"
)

// appClient builds an App client for the configured app base URL. It adds
// the Bearer token to every request.
func (rc *runtime) appClient(ctx context.Context) (*govee.App, error) {
	cfg := rc.cfg
	if cfg.ClientID == "" {
		cfg.ClientID = uuid.NewString()
		if !rc.g.dryRun {
			if err := config.Save(rc.g.configPath, *cfg); err != nil {
				return nil, err
			}
		}
	}
	headers := govee.AppHeaders{
		AppVersion: "7.6.21", ClientID: cfg.ClientID, SysVersion: "31",
		IotVersion: cfg.IotVersion, Language: "en-US", Country: "US",
		TimeZone: "UTC",
	}
	_ = ctx
	return govee.NewApp(cfg.BaseURL, cfg.Token, headers.AppVersion, headers), nil
}

// openAPIClient builds the official API client with the stored API key.
func (rc *runtime) openAPIClient() (*govee.OpenAPI, error) {
	cfg := rc.cfg
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("please set an API key first: %s_API_KEY, config api_key, or goveetl config set api_key <key>", config.EnvPrefix)
	}
	return govee.NewOpenAPI(cfg.OpenAPIBaseURL, cfg.APIKey), nil
}

// saveSession persists login output into config.
func (rc *runtime) saveSession(data *govee.LoginData) error {
	cfg := rc.cfg
	if data.Token != "" {
		cfg.Token = data.Token
	}
	if data.RefreshToken != "" {
		cfg.RefreshToken = data.RefreshToken
	}
	if data.Topic != "" {
		cfg.AccountTopic = data.Topic
	}
	if data.AccountID != 0 {
		cfg.AccountID = data.AccountID
	}
	return config.Save(rc.g.configPath, *cfg)
}

func (rc *runtime) requireToken() error {
	if rc.cfg.Token == "" {
		return fmt.Errorf("app session missing: set GOVEETL_TOKEN or import a captured token with goveetl config set token --stdin")
	}
	return nil
}
