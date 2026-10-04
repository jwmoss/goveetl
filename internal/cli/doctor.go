package cli

import (
	"encoding/json"
	"net/http"

	"github.com/spf13/cobra"
)

func newDoctorCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Verify configuration and API connectivity",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := rc.client.Do(cmd.Context(), http.MethodGet, "/v1/health", nil, nil)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"ok":       true,
				"base_url": rc.cfg.BaseURL,
			}
			if json.Valid(data) && len(data) > 0 {
				var body any
				if err := json.Unmarshal(data, &body); err == nil {
					payload["response"] = body
				}
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(payload)
			}
			rc.out.Success("API reachable")
			rc.out.Printf("base_url: %s\n", rc.cfg.BaseURL)
			return nil
		},
	}
}
