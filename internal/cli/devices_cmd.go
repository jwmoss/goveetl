package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jwmoss/goveetl/internal/config"
	"github.com/jwmoss/goveetl/internal/govee"
)

func newDevicesCommand(rc *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devices",
		Short: "List and inspect Govee devices",
	}
	cmd.AddCommand(newDevicesListCommand(rc))
	cmd.AddCommand(newDevicesStateCommand(rc))
	cmd.AddCommand(newDevicesCapabilitiesCommand(rc))
	return cmd
}

func newScenesCommand(rc *runtime) *cobra.Command {
	var diy bool
	cmd := &cobra.Command{
		Use:   "scenes <device>:<sku>",
		Short: "List official dynamic scenes or saved DIY scenes (--diy)",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseDeviceRef(args[0])
			if err != nil {
				return err
			}
			client, err := rc.openAPIClient()
			if err != nil {
				return err
			}
			result, err := client.Scenes(cmd.Context(), ref.Device, ref.Sku, diy)
			if err != nil {
				return err
			}
			return rc.out.JSON(result)
		},
	}
	cmd.Flags().BoolVar(&diy, "diy", false, "list saved DIY scenes")
	return cmd
}

type deviceRef struct {
	Device string
	Sku    string
}

func parseDeviceRef(args string) (deviceRef, error) {
	// Device ids are MAC-like with colons ("B4:11:D0:C9:07:BF:F5:60"), so
	// the separator must be the LAST colon: B4:11:...:60:H6006.
	idx := strings.LastIndex(args, ":")
	if idx <= 0 || idx == len(args)-1 {
		return deviceRef{}, fmt.Errorf("%w: expected <device>:<sku> like \"B4:11:D0:C9:07:BF:F5:60:H6006\"", errUsage)
	}
	return deviceRef{Device: args[:idx], Sku: args[idx+1:]}, nil
}

func newDevicesListCommand(rc *runtime) *cobra.Command {
	var backend string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every device on the account (backend: api|app, default api)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if backend == "" {
				backend = "api"
			}
			var rows []map[string]any
			switch backend {
			case "api":
				client, err := rc.openAPIClient()
				if err != nil {
					return err
				}
				list, err := client.ListDevices(cmd.Context())
				if err != nil {
					return err
				}
				rows = make([]map[string]any, 0, len(list.Data))
				for _, d := range list.Data {
					rows = append(rows, map[string]any{
						"device":       d.Device,
						"sku":          d.Sku,
						"name":         d.DeviceName,
						"type":         d.Type,
						"capabilities": capabilityInstances(d.Capabilities),
					})
				}
			case "app":
				if err := rc.requireToken(); err != nil {
					return err
				}
				app, err := rc.appClient(cmd.Context())
				if err != nil {
					return err
				}
				list, err := app.DeviceList(cmd.Context())
				if err != nil {
					return err
				}
				rows = make([]map[string]any, 0, len(list.Devices))
				for _, d := range list.Devices {
					rows = append(rows, map[string]any{
						"device":   d.Device,
						"sku":      d.Sku,
						"name":     d.DeviceName,
						"group":    d.GroupID,
						"pact":     fmt.Sprintf("%d/%d", d.PactType, d.PactCode),
						"firmware": strings.TrimSpace(d.VersionHard + " " + d.VersionSoft),
					})
				}
			default:
				return fmt.Errorf("%w: backend must be api|app", errUsage)
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(rows)
			}
			if rc.out.IsPlain() {
				for _, row := range rows {
					rc.out.Printf("%s\t%s\t%s\n", row["device"], row["sku"], row["name"])
				}
				return nil
			}
			rc.out.Printf("%s:\n", pluralizeList(len(rows), "device"))
			for _, row := range rows {
				extra := ""
				if capRow, ok := row["capabilities"]; ok {
					extra = fmt.Sprintf(" capabilities=%v", capRow)
				}
				rc.out.Printf("  %-18s %-8s %-24s%s\n", row["device"], row["sku"], row["name"], extra)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&backend, "backend", "", "list source: api (official OpenAPI) or app (Govee Home app API)")
	return cmd
}

// capabilityInstances flattens the list of capability instances.
func capabilityInstances(caps []govee.OpenAPICapability) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, c.Instance)
	}
	return out
}

func pluralizeList(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func newDevicesStateCommand(rc *runtime) *cobra.Command {
	var backend string
	cmd := &cobra.Command{
		Use:   "state <device>:<sku>",
		Short: "Read the live device state",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseDeviceRef(args[0])
			if err != nil {
				return err
			}
			if backend == "app" {
				return appDeviceState(rc, cmd.Context(), ref)
			}
			client, err := rc.openAPIClient()
			if err != nil {
				return err
			}
			state, err := client.State(cmd.Context(), ref.Device, ref.Sku)
			if err != nil {
				return err
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(state)
			}
			for _, device := range state.DeviceData() {
				for _, capn := range device.Capabilities {
					if capn.State == nil {
						continue
					}
					rc.out.Printf("%-24s %v\n", capn.Instance, capn.State.Value)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&backend, "backend", "", "state source: api or app")
	return cmd
}

func appDeviceState(rc *runtime, ctx context.Context, ref deviceRef) error {
	if err := rc.requireToken(); err != nil {
		return err
	}
	app, err := rc.appClient(ctx)
	if err != nil {
		return err
	}
	matters, err := app.DeviceList(ctx)
	if err != nil {
		return err
	}
	for _, d := range matters.Devices {
		if !strings.EqualFold(d.Device, ref.Device) {
			continue
		}
		if rc.out.IsJSON() {
			return rc.out.JSON(d)
		}
		rc.out.Printf("device:   %s\n", d.DeviceName)
		rc.out.Printf("sku:      %s\n", d.Sku)
		rc.out.Printf("pact:     %d/%d\n", d.PactType, d.PactCode)
		rc.out.Printf("firmware: %s (hard %s)\n", d.VersionSoft, d.VersionHard)
		return nil
	}
	return &govee.Error{Message: "device not in app list: " + ref.Device}
}

func newDevicesCapabilitiesCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "capabilities <device>:<sku>",
		Short: "List official capabilities a device supports (type, instance, parameters)",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseDeviceRef(args[0])
			if err != nil {
				return err
			}
			list, err := rc.openAPIClient()
			if err != nil {
				return err
			}
			devices, err := list.ListDevices(cmd.Context())
			if err != nil {
				return err
			}
			for _, device := range devices.Data {
				if !strings.EqualFold(device.Device, ref.Device) || !strings.EqualFold(device.Sku, ref.Sku) {
					continue
				}
				if rc.out.IsJSON() {
					return rc.out.JSON(device.Capabilities)
				}
				for _, capn := range device.Capabilities {
					rc.out.Printf("%s\t%s\n", capn.Type, capn.Instance)
				}
				return nil
			}
			return &govee.Error{Message: "device not found: " + args[0]}
		},
	}
}

// ControlCmd implements `goveetl control` — official OpenAPI commands plus
// pass-through MQTT envelopes.
func newControlCommand(rc *runtime) *cobra.Command {
	var (
		backend     string
		instance    string
		capTypeFlag string
		version     int
	)
	cmd := &cobra.Command{
		Use: "control <device>:<sku> <name> [value]",
		Short: "Send a control command. Value parses as number, JSON, or chars. " +
			"Commands follow the official capability names (turn, brightness, color, colorTemperature, ...) or raw MQTT envelopes with --backend mqtt",
		Args: usageArgs(cobra.RangeArgs(2, 3)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseDeviceRef(args[0])
			if err != nil {
				return err
			}
			name := args[1]
			var value any
			if len(args) == 3 {
				parsed, perr := parseControlValue(args[2])
				if perr != nil {
					return perr
				}
				value = parsed
			}
			switch backend {
			case "", "api":
				client, err := rc.openAPIClient()
				if err != nil {
					return err
				}
				capType, inst, known := govee.CapRoute(name)
				if instance != "" {
					inst = instance
				}
				if !known && inst == "" {
					return fmt.Errorf("not a known command: %q; use --instance and --capability-type", name)
				}
				if cmd.Flags().Changed("capability-type") {
					capType = capTypeFlag
				}
				return client.Control(cmd.Context(), ref.Device, ref.Sku, capType, inst, value)
			case "mqtt":
				return mqttControl(rc, cmd.Context(), mqttCmdInput{
					Device: ref.Device, Sku: ref.Sku,
					Cmd: name, CmdVersion: version, Value: value,
				})
			default:
				return fmt.Errorf("%w: backend must be api|mqtt", errUsage)
			}
		},
	}
	cmd.Flags().StringVar(&backend, "backend", "", "control channel: api (official) or mqtt (app IOT)")
	cmd.Flags().StringVar(&instance, "instance", "", "capability instance override (e.g. powerSwitch)")
	cmd.Flags().StringVar(&capTypeFlag, "capability-type", "", "capability type override (e.g. devices.capabilities.on_off)")
	cmd.Flags().IntVar(&version, "cmd-version", 0, "MQTT cmdVersion for --backend mqtt (device-specific)")
	return cmd
}

// defaultInstance maps well-known command names to instances (v1 semantics).
func parseControlValue(literal string) (any, error) {
	if n, err := strconv.Atoi(literal); err == nil {
		return n, nil
	}
	if f, err := strconv.ParseFloat(literal, 64); err == nil {
		return f, nil
	}
	if strings.HasPrefix(literal, "{") || strings.HasPrefix(literal, "[") {
		var parsed any
		if err := json.Unmarshal([]byte(literal), &parsed); err != nil {
			return nil, fmt.Errorf("%w: value is neither a number nor JSON", errUsage)
		}
		return parsed, nil
	}
	return literal, nil
}

type mqttCmdInput struct {
	Device, Sku, Cmd string
	CmdVersion       int
	Value            any
}

// mqttControl sends one MQTT control envelope for a wifi device.
func mqttControl(rc *runtime, ctx context.Context, in mqttCmdInput) error {
	if err := rc.requireToken(); err != nil {
		return err
	}
	if _, ok := in.Value.(map[string]any); !ok {
		return fmt.Errorf("%w: MQTT data must be a JSON object, e.g. {\"val\":1}", errUsage)
	}
	envelope, err := govee.WriteEnvelope("", rc.cfg.AccountTopic, in.Cmd, in.CmdVersion, in.Value)
	if err != nil {
		return err
	}
	app, err := rc.appClient(ctx)
	if err != nil {
		return err
	}
	actor := mqttActorID(rc.cfg)
	if actor == "" {
		return fmt.Errorf("mqtt: set account_id and client_id before control")
	}
	cert, err := app.IotCert(ctx)
	if err != nil {
		return err
	}
	topic, err := app.DeviceTopic(ctx, in.Sku, in.Device, rc.cfg.DeviceBaseURL)
	if err != nil {
		return err
	}
	sender := &govee.MqttSender{AccountTopic: rc.cfg.AccountTopic, AccountID: rc.cfg.AccountID, ClientID: actor}
	if err := sender.Connect(cert.Endpoint, []byte(cert.CertificatePem), []byte(cert.PrivateKey)); err != nil {
		return err
	}
	defer sender.Disconnect()
	return sender.SendRaw(topic, envelope)
}

// mqttActorID mirrors the app clientId: "AP/<accountId>/a_<deviceUuid>".
func mqttActorID(cfg *config.Config) string {
	if cfg.AccountID == 0 || cfg.ClientID == "" {
		return ""
	}
	return fmt.Sprintf("AP/%d/a_%s", cfg.AccountID, cfg.ClientID)
}
