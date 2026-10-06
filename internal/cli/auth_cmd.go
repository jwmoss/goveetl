package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/jwmoss/goveetl/internal/config"
	"github.com/jwmoss/goveetl/internal/govee"
)

func newAuthCommand(rc *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage the Govee Home app session",
	}
	cmd.AddCommand(newLoginCommand(rc))
	cmd.AddCommand(newRefreshCommand(rc))
	cmd.AddCommand(newLogoutCommand(rc))
	status := newDoctorCommand(rc)
	status.Use = "status"
	status.Short = "Verify the API key and app session"
	cmd.AddCommand(status)
	return cmd
}

func newLoginCommand(rc *runtime) *cobra.Command {
	var email string
	var fromStdin bool
	login := &cobra.Command{
		Use:   "login",
		Short: "Sign in to Govee Home and save a verified app session",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			email = strings.TrimSpace(email)
			if email == "" {
				email = rc.cfg.Email
			}
			if email == "" {
				return fmt.Errorf("email missing: use --email or set GOVEETL_EMAIL")
			}
			password, err := rc.authSecret(cmd, "Password", "GOVEETL_PASSWORD", fromStdin)
			if err != nil {
				return err
			}
			// Keep a failed first login from writing an empty session to disk.
			if rc.cfg.ClientID == "" {
				rc.cfg.ClientID = uuid.NewString()
			}
			app, err := rc.appClient(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), rc.g.timeout)
			defer cancel()
			code := strings.TrimSpace(os.Getenv("GOVEETL_VERIFICATION_CODE"))
			session, err := app.Login(ctx, email, password, code)
			if errors.Is(err, govee.ErrVerificationRequired) {
				if code != "" {
					return fmt.Errorf("verification code rejected; rerun auth login with a fresh code")
				}
				if err := app.RequestVerification(ctx, email); err != nil {
					return fmt.Errorf("request email verification: %w", err)
				}
				rc.out.Errorf("Govee sends a verification code to your email.\n")
				cancel()
				code, err = rc.authSecret(cmd, "Verification code", "GOVEETL_VERIFICATION_CODE", false)
				if err != nil {
					return err
				}
				ctx, cancel = context.WithTimeout(cmd.Context(), rc.g.timeout)
				defer cancel()
				session, err = app.Login(ctx, email, password, strings.TrimSpace(code))
			}
			if err != nil {
				return err
			}
			rc.cfg.Token = session.Token
			verified, err := rc.appClient(ctx)
			if err != nil {
				return err
			}
			if _, err := verified.DeviceList(ctx); err != nil {
				return fmt.Errorf("verify login session: %w", err)
			}
			rc.cfg.Email = email
			rc.cfg.RefreshToken = ""
			if err := rc.saveSession(session); err != nil {
				return err
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(map[string]bool{"authenticated": true, "saved": true, "refresh_available": session.RefreshToken != ""})
			}
			rc.out.Success("app session verified and saved; use auth status to check access")
			return nil
		},
	}
	login.Flags().StringVar(&email, "email", "", "Govee account email (default: stored email or GOVEETL_EMAIL)")
	login.Flags().BoolVar(&fromStdin, "stdin", false, "read the password from stdin")
	return login
}

func (rc *runtime) authSecret(cmd *cobra.Command, label, env string, fromStdin bool) (string, error) {
	if fromStdin {
		data, err := io.ReadAll(io.LimitReader(rc.stdin, 65537))
		if err != nil {
			return "", err
		}
		if len(data) > 65536 {
			return "", fmt.Errorf("credential input exceeds 64 KiB")
		}
		value := strings.TrimRight(string(data), "\r\n")
		if value == "" {
			return "", fmt.Errorf("%s missing from stdin", label)
		}
		return value, nil
	}
	if value := os.Getenv(env); value != "" {
		return value, nil
	}
	file, ok := rc.stdin.(*os.File)
	if rc.g.noInput || !ok || !term.IsTerminal(int(file.Fd())) {
		hint := "set " + env
		if env == "GOVEETL_PASSWORD" {
			hint += " or use --stdin"
		}
		return "", fmt.Errorf("%s missing: %s", label, hint)
	}
	state, err := term.MakeRaw(int(file.Fd()))
	if err != nil {
		return "", err
	}
	defer term.Restore(int(file.Fd()), state)
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	type result struct {
		data []byte
		err  error
	}
	read := make(chan result, 1)
	rc.out.Errorf("%s: ", label)
	go func() { data, err := term.ReadPassword(int(file.Fd())); read <- result{data, err} }()
	defer rc.out.Errorf("\r\n")
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-read:
		if result.err != nil {
			return "", result.err
		}
		if len(result.data) == 0 {
			return "", fmt.Errorf("%s cannot be empty", label)
		}
		return string(result.data), nil
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
				return fmt.Errorf("token refresh failed: %w; run goveetl auth login to renew the session", err)
			}
			if err := rc.saveSession(data); err != nil {
				return err
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(map[string]string{
					"token": redact(data.Token),
					"topic": "redacted",
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
