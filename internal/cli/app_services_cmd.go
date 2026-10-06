package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/jwmoss/goveetl/internal/govee"
)

func newGroupsCommand(rc *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "groups",
		Short: "Rooms and grouped device control (app API)",
	}
	cmd.AddCommand(newGroupsListCommand(rc))
	cmd.AddCommand(newGroupsDevicesCommand(rc))
	cmd.AddCommand(newGroupsControlCommand(rc))
	return cmd
}

func newGroupsListCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List room groups from general-control",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rc.requireToken(); err != nil {
				return err
			}
			app, err := rc.appClient(cmd.Context())
			if err != nil {
				return err
			}
			data, err := app.GroupList(cmd.Context())
			if err != nil {
				return err
			}
			return rc.emitRawJSON(data)
		},
	}
}

func newGroupsDevicesCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "devices <groupId>",
		Short: "List devices in one general-control group",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rc.requireToken(); err != nil {
				return err
			}
			if _, err := strconv.Atoi(args[0]); err != nil {
				return err
			}
			app, err := rc.appClient(cmd.Context())
			if err != nil {
				return err
			}
			devices, err := app.GroupDevices(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(devices)
			}
			for _, d := range *devices {
				rc.out.Printf("  %-16s %-10s %s\n", d.Device, d.Sku, d.DeviceName)
			}
			return nil
		},
	}
}

func newGroupsControlCommand(rc *runtime) *cobra.Command {
	var sceneID, colorHval int
	cmd := &cobra.Command{
		Use:   "control <groupId> <opType> [sceneId]",
		Short: "Send a REST group/scene control (iot-control)",
		Args:  usageArgs(cobra.RangeArgs(2, 3)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rc.requireToken(); err != nil {
				return err
			}
			groupID, err := strconv.Atoi(args[0])
			if err != nil {
				return err
			}
			op, err := strconv.Atoi(args[1])
			if err != nil {
				return err
			}
			scene := 0
			if len(args) == 3 {
				if scene, err = strconv.Atoi(args[2]); err != nil {
					return err
				}
				_ = sceneID
			}
			req := govee.GroupControlRequest{GroupID: groupID, OpType: op, SceneID: sceneID}
			if len(args) == 3 {
				req.SceneID = scene
			}
			if cmd.Flags().Changed("color-hval") {
				req.ColorHval = &colorHval
			}
			app, err := rc.appClient(cmd.Context())
			if err != nil {
				return err
			}
			if err := app.GroupControl(cmd.Context(), req); err != nil {
				return err
			}
			rc.out.Success("control sent")
			return nil
		},
	}
	cmd.Flags().IntVar(&sceneID, "scene-id", 0, "scene id for scene control")
	cmd.Flags().IntVar(&colorHval, "color-hval", 0, "hue 0-360 for color ops")
	return cmd
}

// emitRawJSON prints a server payload respecting --json/--plain.
func (rc *runtime) emitRawJSON(data []byte) error {
	return rc.out.JSON(json.RawMessage(data))
}

func newLanCommand(rc *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lan",
		Short: "Local UDP control for LAN-capable lights",
	}
	cmd.AddCommand(newLanScanCommand(rc))
	cmd.AddCommand(newLanControlCommand(rc))
	cmd.AddCommand(newLanStatusCommand(rc))
	return cmd
}

func newLanScanCommand(rc *runtime) *cobra.Command {
	var wait time.Duration
	var address string
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Broadcast the LAN scan and print replies",
		RunE: func(cmd *cobra.Command, args []string) error {
			sock := govee.NewLAN()
			ctx, cancel := context.WithTimeout(cmd.Context(), rc.g.timeout)
			defer cancel()
			devices, err := sock.ScanContext(ctx, address, wait)
			if err != nil {
				return err
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(devices)
			}
			rc.out.Printf("%d lan device(s)\n", len(devices))
			for _, d := range devices {
				rc.out.Printf("  %s cmd=%s\n", d.IP, d.Message.Cmd)
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&wait, "wait", 3*time.Second, "listen window")
	cmd.Flags().StringVar(&address, "address", "", "device IPv4 address (default multicast)")
	return cmd
}

func newLanControlCommand(rc *runtime) *cobra.Command {
	var wait time.Duration
	cmd := &cobra.Command{
		Use:   "control <ip> <cmd> <dataJson>",
		Short: "Send one LAN command (e.g. turn, brightness) with JSON object data",
		Args:  usageArgs(cobra.ExactArgs(3)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ip, command, dataLiteral := args[0], args[1], args[2]
			value, err := parseControlValue(dataLiteral)
			if err != nil {
				return err
			}
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("%w: LAN data must be a JSON object, e.g. {\"value\":1}", errUsage)
			}
			sock := govee.NewLAN()
			ctx, cancel := context.WithTimeout(cmd.Context(), rc.g.timeout)
			defer cancel()
			reply, err := sock.ControlContext(ctx, ip, govee.LANMessage{Cmd: command, Data: value}, wait)
			if err != nil {
				return err
			}
			if reply == nil {
				rc.out.Success("sent")
				return nil
			}
			return rc.emitRawJSON(reply)
		},
	}
	cmd.Flags().DurationVar(&wait, "wait", 1500*time.Millisecond, "reply wait")
	return cmd
}

func newLanStatusCommand(rc *runtime) *cobra.Command {
	var wait time.Duration
	cmd := &cobra.Command{
		Use:   "status <ip>",
		Short: "Read device state directly over LAN",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), rc.g.timeout)
			defer cancel()
			reply, err := govee.NewLAN().ControlContext(ctx, args[0], govee.LANMessage{Cmd: "devStatus"}, wait)
			if err != nil {
				return err
			}
			return rc.emitRawJSON(reply)
		},
	}
	cmd.Flags().DurationVar(&wait, "wait", 1500*time.Millisecond, "reply wait")
	return cmd
}
