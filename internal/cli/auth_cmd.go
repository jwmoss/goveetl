package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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
	var email, password, code string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in with the Govee Home app account and store the token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				email = rc.cfg.Email
			}
			if password == "" {
				password = os.Getenv(config.EnvPrefix + "_PASSWORD")
			}
			if rc.g.noInput {
				return fmt.Errorf("%w: --password required with --no-input", errUsage)
			}
			if password == "" {
				entered, err := rc.promptPassword(cmd, "Password: ")
				if err != nil {
					return err
				}
				password = entered
			}
			app, err := rc.appClient(rc.ctx)
			if err != nil {
				return err
			}
			data, err := app.Login(rc.ctx, strings.TrimSpace(email), password, strings.TrimSpace(code))
			if err != nil {
				return err
			}
			if err := rc.saveSession(data); err != nil {
				return err
			}
			payload := map[string]any{
				"account_id":    data.AccountID,
				"topic":         data.Topic,
				"token":         redact(data.Token),
				"refresh_token": redact(data.RefreshToken),
				"expire_cycle":  data.TokenExpireCycle,
				"email":         data.Email,
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(payload)
			}
			rc.out.Success("login ok")
			rc.out.Printf("account_id: %d\n", data.AccountID)
			rc.out.Printf("topic: %s\n", data.Topic)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "account email (default config email)")
	cmd.Flags().StringVar(&password, "password", "", "account password (default GOVEETL_PASSWORD)")
	cmd.Flags().StringVar(&code, "code", "", "verification code for new-device login")
	return cmd
}

func newRefreshCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Rotate the stored session token using the refresh token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if rc.cfg.RefreshToken == "" {
				return fmt.Errorf("refresh token missing: run goveetl login first")
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
	if len(token) <= 12 {
		return "redacted"
	}
	return token[:8] + "…" + token[len(token)-4:]
}

// promptPassword reads a password from stdin.
// ponytail: terminal echo-off needs syscall plumbing; stdin is fine for a CLI.
func (rc *runtime) promptPassword(_ *cobra.Command, prompt string) (string, error) {
	if !rc.g.quiet {
		_, _ = fmt.Fprint(rc.stderr, prompt)
	}
	reader := bufio.NewReader(rc.stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read password: %w", err)
	}
	if !rc.g.quiet {
		_, _ = fmt.Fprintln(rc.stderr)
	}
	return strings.TrimSpace(line), nil
}
