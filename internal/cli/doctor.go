package cli

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/jwmoss/goveetl/internal/api"
)

func newDoctorCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Verify configuration and API connectivity",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ctx, cancel := context.WithTimeout(ctx, rc.g.timeout)
			defer cancel()

			report := map[string]any{}
			report["openapi"] = doctorCheckOpenAPI(rc, ctx)
			report["app"] = doctorCheckApp(rc, ctx)
			if rc.out.IsJSON() {
				return rc.out.JSON(report)
			}
			rc.out.Printf("base_url:    %s\n", rc.cfg.BaseURL)
			rc.out.Printf("openapi_url: %s\n", rc.cfg.OpenAPIBaseURL)
			if apiKeyConfigured(rc) {
				rc.out.Printf("openapi key: present\n")
			} else {
				rc.out.Printf("openapi key: MISSING (set api_key)\n")
			}
			if state := report["app"].(doctorAppResult); state.Token {
				rc.out.Printf("app session: present\n")
			} else {
				rc.out.Printf("app session: MISSING (run goveetl auth login --email <email>)\n")
			}
			for _, item := range []struct {
				name   string
				result doctorResult
			}{
				{"openapi", report["openapi"].(doctorResult)}, {"app", report["app"].(doctorAppResult).Doctor},
			} {
				if item.result.Error != "" {
					rc.out.Printf("%s check: %s\n", item.name, item.result.Error)
				} else if item.result.Configured {
					rc.out.Printf("%s check: authenticated\n", item.name)
				}
			}
			return nil
		},
	}
}

func apiKeyConfigured(rc *runtime) bool { return rc.cfg.APIKey != "" }

// doctorCheckOpenAPI proves the key works with a cheap authenticated call.
func doctorCheckOpenAPI(rc *runtime, ctx context.Context) doctorResult {
	out := doctorResult{Configured: rc.cfg.APIKey != ""}
	if !out.Configured {
		return out
	}
	client := rc.diagnosticClient(rc.cfg.OpenAPIBaseURL, "Govee-API-Key", "", rc.cfg.APIKey)
	start := time.Now()
	_, err := client.Do(ctx, http.MethodGet, "/router/api/v1/user/devices", nil, nil)
	out = fin(out, err, start)
	return out
}

// doctorCheckApp proves the token authenticates the app device list.
func doctorCheckApp(rc *runtime, ctx context.Context) doctorAppResult {
	result := doctorAppResult{Token: rc.cfg.Token != ""}
	if !result.Token {
		return result
	}
	app, err := rc.appClient(ctx)
	if err != nil {
		result.Doctor = doctorResult{Configured: true, Error: err.Error()}
		return result
	}
	start := time.Now()
	_, err = app.DeviceList(ctx)
	result.Doctor = fin(doctorResult{Configured: true}, err, start)
	return result
}

type doctorResult struct {
	Configured bool   `json:"configured"`
	Reachable  bool   `json:"reachable"`
	Status     int    `json:"status"`
	Error      string `json:"error,omitempty"`
}

type doctorAppResult struct {
	Token  bool         `json:"token"`
	Doctor doctorResult `json:"doctor"`
}

func fin(started doctorResult, err error, _ time.Time) doctorResult {
	if err == nil {
		started.Reachable = true
		started.Status = http.StatusOK
		return started
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.Status > 0 {
		started.Status = apiErr.Status
		// A 4xx from the service still proves the endpoint answers.
		started.Reachable = apiErr.Status >= 200
		started.Error = apiErr.Error()
		return started
	}
	started.Error = err.Error()
	return started
}

// diagnosticClient applies the selected timeout and trace options.
func (rc *runtime) diagnosticClient(baseURL, header, scheme, token string) *api.Client {
	options := append([]api.Option{api.WithAuth(header, scheme, token)}, rc.providerOptions()...)
	return api.New(baseURL, options...)
}
