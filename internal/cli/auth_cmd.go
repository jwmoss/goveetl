package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jwmoss/goveetl/internal/config"
)

func newAuthCommand(rc *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage the Govee Home app session",
	}
	cmd.AddCommand(newLoginCommand(rc))
	cmd.AddCommand(newRefreshCommand(rc))
	cmd.AddCommand(newLogoutCommand(rc))
	return cmd
}

func newLoginCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Explain how to import a captured Govee Home session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("encrypted password login is not supported; import a captured token with goveetl config set token --stdin, then set account_id and account_topic")
		},
	}
}

func newRefreshCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Rotate the stored session token using the refresh token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if rc.cfg.RefreshToken == "" {
				return fmt.Errorf("refresh token missing: import a captured refresh_token with goveetl config set refresh_token --stdin")
			}
			app, err := rc.appClient(rc.ctx)
			if err != nil {
				return err
			}
			data, err := app.Refresh(rc.ctx, rc.cfg.RefreshToken)
			if err != nil {
				return err
			}
			if err := rc.saveSession(data); err != nil {
				return err
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(map[string]string{
					"token": redact(data.Token),
					"topic": data.Topic,
				})
			}
			rc.out.Success("token refreshed")
			return nil
		},
	}
}

func newLogoutCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear the stored app session",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := *rc.cfg
			cfg.Token = ""
			cfg.RefreshToken = ""
			cfg.AccountTopic = ""
			cfg.AccountID = 0
			if err := config.Save(rc.g.configPath, cfg); err != nil {
				return err
			}
			rc.out.Success("session cleared")
			return nil
		},
	}
}

func redact(token string) string {
	if token == "" {
		return ""
	}
	return "redacted"
}
