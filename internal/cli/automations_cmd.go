package cli

import (
	"fmt"
	"strconv"

	"github.com/jwmoss/goveetl/internal/govee"
	"github.com/spf13/cobra"
)

func newAutomationsCommand(rc *runtime) *cobra.Command {
	root := &cobra.Command{Use: "automations", Short: "Read and update saved Govee Home automations"}
	list := &cobra.Command{Use: "list", Short: "List automation IDs, names, and enabled state", Args: usageArgs(cobra.NoArgs), RunE: func(cmd *cobra.Command, args []string) error {
		if err := rc.requireToken(); err != nil {
			return err
		}
		app, err := rc.appClient(cmd.Context())
		if err != nil {
			return err
		}
		items, err := app.ListAutomations(cmd.Context())
		if err != nil {
			return err
		}
		return rc.out.JSON(items)
	}}
	show := &cobra.Command{Use: "show <id>", Short: "Read saved device settings without private MQTT data", Args: usageArgs(cobra.ExactArgs(1)), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := automationID(args[0])
		if err != nil {
			return err
		}
		if err := rc.requireToken(); err != nil {
			return err
		}
		app, err := rc.appClient(cmd.Context())
		if err != nil {
			return err
		}
		view, err := app.ReadAutomation(cmd.Context(), id)
		if err != nil {
			return err
		}
		return rc.out.JSON(view)
	}}
	remove := &cobra.Command{Use: "remove-device <id> <device>:<sku>", Short: "Remove a device action (experimental); verify the saved result", Args: usageArgs(cobra.ExactArgs(2)), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := automationID(args[0])
		if err != nil {
			return err
		}
		ref, err := parseDeviceRef(args[1])
		if err != nil {
			return err
		}
		if err := rc.requireToken(); err != nil {
			return err
		}
		app, err := rc.appClient(cmd.Context())
		if err != nil {
			return err
		}
		view, err := app.RemoveAutomationDevice(cmd.Context(), id, ref.Device, ref.Sku)
		if err != nil {
			return err
		}
		return rc.out.JSON(view)
	}}
	var power string
	var brightness, temperature int
	set := &cobra.Command{Use: "set <id> <device>:<sku>", Short: "Set saved light settings (experimental); verify the result", Args: usageArgs(cobra.ExactArgs(2)), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := automationID(args[0])
		if err != nil {
			return err
		}
		ref, err := parseDeviceRef(args[1])
		if err != nil {
			return err
		}
		settings := govee.AutomationLightSettings{}
		if cmd.Flags().Changed("power") {
			n := 0
			switch power {
			case "on":
				n = 1
			case "off":
			default:
				return fmt.Errorf("%w: power must be on or off", errUsage)
			}
			settings.Power = &n
		}
		if cmd.Flags().Changed("brightness") {
			settings.Brightness = &brightness
		}
		if cmd.Flags().Changed("temperature") {
			settings.TemperatureK = &temperature
		}
		if err := rc.requireToken(); err != nil {
			return err
		}
		app, err := rc.appClient(cmd.Context())
		if err != nil {
			return err
		}
		view, err := app.SetAutomationLight(cmd.Context(), id, ref.Device, ref.Sku, settings)
		if err != nil {
			return err
		}
		return rc.out.JSON(view)
	}}
	set.Flags().StringVar(&power, "power", "", "saved power: on or off")
	set.Flags().IntVar(&brightness, "brightness", 0, "saved brightness: 1-100")
	set.Flags().IntVar(&temperature, "temperature", 0, "saved native warm white: H706C at 2700 K")
	root.AddCommand(list, show, set, remove)
	return root
}

func automationID(value string) (int, error) {
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: automation ID must be positive", errUsage)
	}
	return id, nil
}
