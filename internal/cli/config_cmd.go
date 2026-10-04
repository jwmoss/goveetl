package cli

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jwmoss/goveetl/internal/config"
)

func newConfigCommand(rc *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and initialize configuration",
	}
	cmd.AddCommand(newConfigShowCommand(rc))
	cmd.AddCommand(newConfigInitCommand(rc))
	cmd.AddCommand(newConfigSetCommand(rc))
	return cmd
}

// settable keys for `config set` — one logical edit per key.
var settableConfigKeys = []string{
	"base_url", "openapi_base_url", "device_base_url", "api_key",
	"token", "refresh_token", "account_topic", "account_id",
	"client_id", "email", "iot_version", "lan_key",
}

func newConfigSetCommand(rc *runtime) *cobra.Command {
	var fromStdin bool
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set one config key (api_key, lan_key, email, ...)",
		Args:  usageArgs(cobra.ExactArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			if fromStdin {
				in, readErr := io.ReadAll(cmd.InOrStdin())
				if readErr != nil {
					return fmt.Errorf("read value from stdin: %w", readErr)
				}
				value = strings.TrimSpace(string(in))
			}
			if !slices.Contains(settableConfigKeys, key) {
				return fmt.Errorf("%w: unknown key %q; settable: %s", errUsage, key, strings.Join(settableConfigKeys, ", "))
			}
			cfg, loadErr := config.Load(rc.g.configPath)
			if loadErr != nil {
				return loadErr
			}
			if applyErr := setConfigKey(cfg, key, value); applyErr != nil {
				return applyErr
			}
			if saveErr := config.Save(rc.g.configPath, *cfg); saveErr != nil {
				return saveErr
			}
			rc.out.Success("set " + key)
			return nil
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read value from stdin instead of argv (for secrets)")
	return cmd
}

func setConfigKey(cfg *config.Config, key, value string) error {
	switch key {
	case "base_url":
		cfg.BaseURL = value
	case "openapi_base_url":
		cfg.OpenAPIBaseURL = value
	case "device_base_url":
		cfg.DeviceBaseURL = value
	case "api_key":
		cfg.APIKey = value
	case "token":
		cfg.Token = value
	case "refresh_token":
		cfg.RefreshToken = value
	case "account_topic":
		cfg.AccountTopic = value
	case "account_id":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("account_id must be a number")
		}
		cfg.AccountID = parsed
	case "client_id":
		cfg.ClientID = value
	case "email":
		cfg.Email = value
	case "iot_version":
		cfg.IotVersion = value
	case "lan_key":
		cfg.LANKey = value
	}
	return nil
}

func newConfigShowCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show effective configuration with secrets redacted",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(rc.g.configPath)
			if err != nil {
				return err
			}
			if rc.g.baseURL != "" {
				cfg.BaseURL = rc.g.baseURL
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(cfg.Redacted())
			}
			rc.out.Table([]string{"KEY", "VALUE"}, [][]string{
				{"base_url", cfg.BaseURL},
				{"token", cfg.Redacted()["token"]},
				{"auth_header", cfg.AuthHeader},
				{"auth_scheme", cfg.AuthScheme},
				{"path", config.DefaultPath()},
			})
			return nil
		},
	}
}

func newConfigInitCommand(rc *runtime) *cobra.Command {
	var (
		baseURL    string
		tokenStdin bool
		force      bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := rc.g.configPath
			if path == "" {
				path = config.DefaultPath()
			}
			if !force && fileExists(path) {
				return fmt.Errorf("config already exists at %s; use --force to overwrite", path)
			}
			cfg := config.Default()
			if baseURL != "" {
				cfg.BaseURL = baseURL
			}
			if tokenStdin {
				data, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read token from stdin: %w", err)
				}
				cfg.Token = strings.TrimSpace(string(data))
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			rc.out.Success("config written")
			rc.out.Printf("%s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&baseURL, "base-url", config.DefaultBaseURL, "API base URL")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "read token from stdin and store it in the config file")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config file")
	return cmd
}
