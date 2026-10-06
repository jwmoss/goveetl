package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newRawCommand(rc *runtime) *cobra.Command {
	var (
		dataFlag  string
		fileFlag  string
		queryFlag []string
		backend   string
	)
	cmd := &cobra.Command{
		Use:   "raw <method> <path>",
		Short: "Send a raw HTTP request",
		Args:  usageArgs(cobra.ExactArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if rc.g.dryRun && !strings.EqualFold(strings.TrimSpace(args[0]), http.MethodGet) {
				return fmt.Errorf("dry-run: refusing %s %s", args[0], args[1])
			}
			var body any
			if dataFlag != "" && fileFlag != "" {
				return fmt.Errorf("%w: use only one of --data or --file", errUsage)
			}
			if dataFlag != "" {
				if err := json.Unmarshal([]byte(dataFlag), &body); err != nil {
					return fmt.Errorf("parse --data JSON: %w", err)
				}
			}
			if fileFlag != "" {
				data, err := os.ReadFile(fileFlag)
				if err != nil {
					return fmt.Errorf("read --file: %w", err)
				}
				if err := json.Unmarshal(data, &body); err != nil {
					return fmt.Errorf("parse --file JSON: %w", err)
				}
			}
			query, err := parseQuery(queryFlag)
			if err != nil {
				return err
			}
			var resp []byte
			switch backend {
			case "", "raw":
				resp, err = rc.client.Do(cmd.Context(), args[0], args[1], query, body)
			case "app":
				if err := rc.requireToken(); err != nil {
					return err
				}
				app, e := rc.appClient(cmd.Context())
				if e != nil {
					return e
				}
				resp, err = app.Do(cmd.Context(), args[0], args[1], query, body)
			default:
				return fmt.Errorf("%w: backend must be raw or app", errUsage)
			}
			if err != nil {
				return err
			}
			if rc.out.IsJSON() && json.Valid(resp) {
				var decoded any
				if err := json.Unmarshal(resp, &decoded); err != nil {
					return err
				}
				return rc.out.JSON(decoded)
			}
			if len(resp) > 0 {
				rc.out.Printf("%s", string(resp))
				if !strings.HasSuffix(string(resp), "\n") {
					rc.out.Printf("\n")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dataFlag, "data", "", "JSON request body")
	cmd.Flags().StringVar(&fileFlag, "file", "", "path to JSON request body")
	cmd.Flags().StringArrayVar(&queryFlag, "query", nil, "query parameter in key=value form")
	cmd.Flags().StringVar(&backend, "backend", "", "request headers: raw (default) or app")
	_ = http.MethodGet
	return cmd
}

func parseQuery(items []string) (url.Values, error) {
	values := url.Values{}
	for _, item := range items {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("%w: query must be key=value", errUsage)
		}
		values.Add(key, value)
	}
	return values, nil
}
