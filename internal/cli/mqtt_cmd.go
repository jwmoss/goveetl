package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/jwmoss/goveetl/internal/govee"
)

func newMqttCommand(rc *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mqtt",
		Short: "Resolve and observe the app's AWS IoT control channel",
	}
	cmd.AddCommand(newMqttCertCommand(rc))
	cmd.AddCommand(newMqttTopicCommand(rc))
	cmd.AddCommand(newMqttWatchCommand(rc))
	return cmd
}

func newMqttCertCommand(rc *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "cert",
		Short: "Fetch the MQTT certificate bundle (endpoint + cert + key)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rc.requireToken(); err != nil {
				return err
			}
			app, err := rc.appClient(cmd.Context())
			if err != nil {
				return err
			}
			cert, err := app.IotCert(cmd.Context())
			if err != nil {
				return err
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(map[string]string{
					"endpoint": cert.Endpoint,
					"cert_sha": shortFingerprint(cert.CertificatePem),
					"key_sha":  shortFingerprint(cert.PrivateKey),
				})
			}
			rc.out.Printf("endpoint: %s\n", cert.Endpoint)
			rc.out.Printf("cert:     %s\n", shortFingerprint(cert.CertificatePem))
			rc.out.Printf("key:      %s\n", shortFingerprint(cert.PrivateKey))
			return nil
		},
	}
}

func newMqttTopicCommand(rc *runtime) *cobra.Command {
	var device, sku string
	cmd := &cobra.Command{
		Use:   "topic",
		Short: "Resolve the MQTT publish topic for one device",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rc.requireToken(); err != nil {
				return err
			}
			app, err := rc.appClient(cmd.Context())
			if err != nil {
				return err
			}
			cert, err := app.IotCert(cmd.Context())
			if err != nil {
				return err
			}
			topic := rc.cfg.AccountTopic
			if device != "" && sku != "" {
				resolved, terr := app.DeviceTopic(cmd.Context(), sku, device, rc.cfg.DeviceBaseURL)
				if terr != nil {
					return terr
				}
				topic = resolved.AccountTopic
			}
			payload := map[string]any{
				"account_topic": rc.cfg.AccountTopic,
				"device_topic":  topic,
				"endpoint":      cert.Endpoint,
				"client_id":     mqttActorID(rc.cfg),
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(payload)
			}
			for _, key := range []string{"account_topic", "device_topic", "endpoint", "client_id"} {
				rc.out.Printf("%s: %v\n", key, payload[key])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&device, "device", "", "device id (MAC-style)")
	cmd.Flags().StringVar(&sku, "sku", "", "device sku (e.g. H6123)")
	cmd.MarkFlagsRequiredTogether("device", "sku")
	return cmd
}

func newMqttWatchCommand(rc *runtime) *cobra.Command {
	var (
		duration time.Duration
		endpoint string
	)
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Connect MQTT, subscribe to the account topic, and stream state pushes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := rc.requireToken(); err != nil {
				return err
			}
			var mqttEndpoint string
			app, err := rc.appClient(ctx)
			if err != nil {
				return err
			}
			cert, cerr := app.IotCert(ctx)
			if cerr == nil {
				mqttEndpoint = cert.Endpoint
			}
			if endpoint != "" {
				mqttEndpoint = endpoint
			}
			if mqttEndpoint == "" {
				apiErr := cerr
				if apiErr == nil {
					apiErr = fmt.Errorf("no endpoint")
				}
				return fmt.Errorf("mqtt endpoint unavailable: %w", apiErr)
			}
			actor := mqttActorID(rc.cfg)
			if actor == "" {
				return fmt.Errorf("mqtt client id incomplete: run goveetl auth login (account id + device uuid)")
			}
			sender := &govee.MqttSender{
				AccountTopic: rc.cfg.AccountTopic,
				AccountID:    rc.cfg.AccountID,
				ClientID:     actor,
			}
			connectErr := sender.Connect(mqttEndpoint, []byte(cert.CertificatePem), []byte(cert.PrivateKey), rc.cfg.AccountTopic)
			if connectErr != nil {
				return connectErr
			}
			defer sender.Disconnect()
			rc.out.Success("connected to " + mqttEndpoint)
			timeout := time.After(duration)
			for {
				select {
				case <-timeout:
					return nil
				case message := <-sender.Watch():
					line := string(message.Payload())
					if !json.Valid([]byte(line)) {
						line = fmt.Sprintf("%q", line)
					}
					rc.out.Printf("topic=%s %s\n", message.Topic(), line)
				case <-ctx.Done():
					return nil
				}
			}
		},
	}
	cmd.Flags().DurationVar(&duration, "duration", 30*time.Second, "stream window")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "MQTT endpoint override (ssl://host:port)")
	return cmd
}

func shortFingerprint(pem string) string {
	if len(pem) < 16 {
		return ""
	}
	return pem[:16] + "…" + fmt.Sprint(len(pem))
}
