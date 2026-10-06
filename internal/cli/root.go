package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jwmoss/goveetl/internal/api"
	"github.com/jwmoss/goveetl/internal/config"
	"github.com/jwmoss/goveetl/internal/output"
)

const (
	exitOK    = 0
	exitErr   = 1
	exitUsage = 2
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

type globals struct {
	configPath  string
	baseURL     string
	asJSON      bool
	plain       bool
	quiet       bool
	noColor     bool
	showVersion bool
	timeout     time.Duration
	traceHTTP   bool
	dryRun      bool
	noInput     bool
}

type runtime struct {
	ctx    context.Context
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	g      *globals
	cfg    *config.Config
	out    *output.Formatter
	client *api.Client
}

func Execute(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	rc := &runtime{
		ctx:    ctx,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		g:      &globals{timeout: 30 * time.Second},
	}

	cmd := newRootCommand(rc)
	cmd.SetArgs(args)
	cmd.SetIn(stdin)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err := cmd.ExecuteContext(ctx); err != nil {
		if !errors.Is(err, errSilent) {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		}
		if errors.Is(err, errUsage) {
			return exitUsage
		}
		return exitErr
	}
	return exitOK
}

func newRootCommand(rc *runtime) *cobra.Command {
	root := &cobra.Command{
		Use:           "goveetl",
		Short:         "Command-line client for Govee cloud, app, and LAN APIs",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if rc.g.showVersion {
				return rc.writeVersion()
			}
			_ = cmd.Help()
			return errUsage
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if rc.g.dryRun {
				switch cmd.Name() {
				case "control", "login", "refresh", "logout", "set", "init", "remove-device":
					return fmt.Errorf("dry-run: refusing %s", cmd.CommandPath())
				}
			}
			if commandSkipsClient(cmd) {
				rc.out = output.New(rc.stdout, rc.stderr, rc.g.asJSON, rc.g.plain, rc.g.quiet, rc.g.noColor)
				return nil
			}
			return rc.initClient()
		},
	}

	flags := root.PersistentFlags()
	flags.StringVar(&rc.g.configPath, "config", "", "config file path")
	flags.StringVar(&rc.g.baseURL, "base-url", "", "API base URL override")
	flags.BoolVar(&rc.g.asJSON, "json", false, "emit JSON to stdout")
	flags.BoolVar(&rc.g.plain, "plain", false, "emit stable plain text where available")
	flags.BoolVarP(&rc.g.quiet, "quiet", "q", false, "suppress non-essential output")
	flags.BoolVar(&rc.g.noColor, "no-color", false, "disable color")
	flags.BoolVar(&rc.g.showVersion, "version", false, "print version and exit")
	flags.DurationVar(&rc.g.timeout, "timeout", 30*time.Second, "HTTP timeout")
	flags.BoolVar(&rc.g.traceHTTP, "trace-http", false, "log HTTP requests to stderr without secrets")
	flags.BoolVar(&rc.g.dryRun, "dry-run", false, "refuse mutations and non-GET raw requests")
	flags.BoolVar(&rc.g.noInput, "no-input", false, "disable interactive prompts")

	root.AddCommand(newVersionCommand(rc))
	root.AddCommand(newConfigCommand(rc))
	root.AddCommand(newDoctorCommand(rc))
	root.AddCommand(newAuthCommand(rc))
	root.AddCommand(newRawCommand(rc))
	root.AddCommand(newDevicesCommand(rc))
	root.AddCommand(newScenesCommand(rc))
	root.AddCommand(newControlCommand(rc))
	root.AddCommand(newGroupsCommand(rc))
	root.AddCommand(newAutomationsCommand(rc))
	root.AddCommand(newMqttCommand(rc))
	root.AddCommand(newLanCommand(rc))
	root.AddCommand(newCompletionCommand(root))

	return root
}

func (rc *runtime) initClient() error {
	if rc.g.asJSON && rc.g.plain {
		return fmt.Errorf("%w: choose only one of --json or --plain", errUsage)
	}
	cfg, err := config.Load(rc.g.configPath)
	if err != nil {
		return err
	}
	if rc.g.baseURL != "" {
		cfg.BaseURL = rc.g.baseURL
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	rc.cfg = cfg
	rc.out = output.New(rc.stdout, rc.stderr, rc.g.asJSON, rc.g.plain, rc.g.quiet, rc.g.noColor)
	options := []api.Option{
		api.WithTimeout(rc.g.timeout),
		api.WithAuth(cfg.AuthHeader, cfg.AuthScheme, cfg.Token),
		api.WithDryRun(rc.g.dryRun),
		api.WithUserAgent("goveetl/" + version),
	}
	if rc.g.traceHTTP {
		options = append(options, api.WithTrace(func(method, path string, status int, duration time.Duration) {
			_, _ = fmt.Fprintf(rc.stderr, "[http] %s %s -> %d (%s)\n", method, path, status, duration)
		}))
	}
	rc.client = api.New(cfg.BaseURL, options...)
	return nil
}

func commandSkipsClient(cmd *cobra.Command) bool {
	for cmd != nil {
		switch cmd.Name() {
		case "completion", "config", "help", "version":
			return true
		}
		cmd = cmd.Parent()
	}
	return false
}

func newVersionCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rc.writeVersion()
		},
	}
}

func (rc *runtime) writeVersion() error {
	payload := map[string]string{
		"version": version,
		"commit":  commit,
		"date":    date,
	}
	if rc.out.IsJSON() {
		return rc.out.JSON(payload)
	}
	if rc.out.IsPlain() {
		rc.out.Printf("%s\n", version)
		return nil
	}
	rc.out.Printf("goveetl version %s\n", version)
	rc.out.Printf("commit: %s\n", commit)
	rc.out.Printf("built:  %s\n", date)
	return nil
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return root.GenPowerShellCompletion(cmd.OutOrStdout())
			default:
				return fmt.Errorf("%w: unsupported shell %q", errUsage, args[0])
			}
		},
	}
	return cmd
}

var (
	errUsage  = errors.New("invalid usage")
	errSilent = errors.New("silent")
)

func usageArgs(fn cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := fn(cmd, args); err != nil {
			return fmt.Errorf("%w: %v", errUsage, err)
		}
		return nil
	}
}

func apiExitCode(err error) int {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden {
			return exitErr
		}
	}
	return exitErr
}

func init() {
	cobra.EnableCommandSorting = false
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
