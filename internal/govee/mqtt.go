package govee

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MqttSender sends control messages over the app's AWS IoT MQTT channel.
//
// Evidence (base2home/iot/Write.java, Govee Home 7.6.21):
//
//	{"msg":{"transaction":"..","accountTopic":"..","cmd":"..","cmdVersion":0,
//	 "data":{...},"type":1,"origin":32}}
//
// clientId = "AP/<accountId>/a_<deviceUuid>"; endpoint and the mutual-TLS
// client certificate come from GET app/v1/account/iot/key.
type MqttSender struct {
	AccountTopic string
	AccountID    int
	ClientID     string

	client   mqtt.Client
	messages chan mqtt.Message
}

// IotTransaction produces a timestamp transaction ID for MQTT writes.
func IotTransaction() string { return fmt.Sprintf("v_%d", time.Now().UnixMicro()) }

// WriteEnvelope builds the app write envelope.
func WriteEnvelope(transaction, accountTopic, cmd string, cmdVersion int, data any) (map[string]any, error) {
	if accountTopic == "" {
		return nil, fmt.Errorf("mqtt: set account_topic before control")
	}
	if transaction == "" {
		transaction = IotTransaction()
	}
	return map[string]any{"msg": map[string]any{
		"transaction":  transaction,
		"accountTopic": accountTopic,
		"cmd":          cmd,
		"cmdVersion":   cmdVersion,
		"data":         data,
		"type":         1,
		"origin":       32,
	}}, nil
}

// Connect establishes the MQTT session and optionally subscribes to topics.
func (m *MqttSender) Connect(endpoint string, certificatePem, privateKeyPem []byte, subscribe ...string) error {
	return m.ConnectContext(context.Background(), endpoint, certificatePem, privateKeyPem, subscribe...)
}
func (m *MqttSender) ConnectContext(ctx context.Context, endpoint string, certificatePem, privateKeyPem []byte, subscribe ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if endpoint == "" {
		return fmt.Errorf("mqtt: endpoint required (fetch app/v1/account/iot/key)")
	}
	if !strings.Contains(endpoint, "://") {
		if _, _, err := net.SplitHostPort(endpoint); err != nil {
			endpoint = net.JoinHostPort(endpoint, "8883")
		}
		endpoint = "ssl://" + endpoint
	}
	cert, err := tls.X509KeyPair(certificatePem, privateKeyPem)
	if err != nil {
		return fmt.Errorf("mqtt: parse certificate: %w", err)
	}
	opts := mqtt.NewClientOptions().
		AddBroker(endpoint).
		SetClientID(m.ClientID).
		SetTLSConfig(&tls.Config{Certificates: []tls.Certificate{cert}}).
		SetConnectTimeout(mqttWait(ctx, 15*time.Second)).
		SetWriteTimeout(mqttWait(ctx, 10*time.Second)).
		SetCleanSession(true).
		SetAutoReconnect(false).
		SetKeepAlive(120 * time.Second)
	if m.messages == nil {
		m.messages = make(chan mqtt.Message, 64)
	}
	client := mqtt.NewClient(opts)
	token := client.Connect()
	if err := waitMQTT(ctx, token, 20*time.Second); err != nil {
		client.Disconnect(0)
		return fmt.Errorf("mqtt: connect timeout: %w", err)
	}
	if token.Error() != nil {
		client.Disconnect(0)
		return fmt.Errorf("mqtt: connect: %w", token.Error())
	}
	m.client = client
	for _, sub := range subscribe {
		if sub == "" {
			continue
		}
		tok := client.Subscribe(sub, 0, m.handler)
		if err := waitMQTT(ctx, tok, 10*time.Second); err != nil {
			m.Disconnect()
			return fmt.Errorf("mqtt: subscribe timeout: %w", err)
		}
		if tok.Error() != nil {
			m.Disconnect()
			return fmt.Errorf("mqtt: subscribe %s: %w", sub, tok.Error())
		}
	}
	return nil
}

func (m *MqttSender) handler(_ mqtt.Client, msg mqtt.Message) {
	if m.messages != nil {
		select {
		case m.messages <- msg:
		default: // ponytail: watch is best-effort, drop when nobody reads
		}
	}
}

// Write sends a typed control command to the account topic.
func (m *MqttSender) Write(cmd string, cmdVersion int, data any) error {
	envelope, err := WriteEnvelope(IotTransaction(), m.AccountTopic, cmd, cmdVersion, data)
	if err != nil {
		return err
	}
	return m.publish(m.AccountTopic, envelope)
}

// SendRaw publishes an arbitrary message to a topic (raw escape hatch).
func (m *MqttSender) SendRaw(topic string, message any) error {
	return m.SendRawContext(context.Background(), topic, message)
}
func (m *MqttSender) SendRawContext(ctx context.Context, topic string, message any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if topic == "" {
		return fmt.Errorf("mqtt: topic required")
	}
	return m.publishContext(ctx, topic, message)
}

func (m *MqttSender) publish(topic string, message any) error {
	return m.publishContext(context.Background(), topic, message)
}
func (m *MqttSender) publishContext(ctx context.Context, topic string, message any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.client == nil {
		return fmt.Errorf("mqtt: not connected")
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("mqtt: encode message: %w", err)
	}
	token := m.client.Publish(topic, 0, false, payload)
	if err := waitMQTT(ctx, token, 10*time.Second); err != nil {
		return fmt.Errorf("mqtt: publish timeout: %w", err)
	}
	if token.Error() != nil {
		return fmt.Errorf("mqtt: publish %s: %w", topic, token.Error())
	}
	return nil
}

// Watch returns received state pushes until Disconnect. Reading it is
// optional; unread messages are dropped.
func (m *MqttSender) Watch() <-chan mqtt.Message {
	if m.messages == nil {
		m.messages = make(chan mqtt.Message, 64)
	}
	return m.messages
}

// Disconnect tears the session down.
func (m *MqttSender) Disconnect() {
	if m.client != nil {
		m.client.Disconnect(500)
		m.client = nil
	}
}

func mqttWait(ctx context.Context, fallback time.Duration) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		if left := time.Until(deadline); left < fallback {
			if left <= 0 {
				return time.Nanosecond
			}
			return left
		}
	}
	return fallback
}
func waitMQTT(ctx context.Context, token mqtt.Token, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return context.DeadlineExceeded
	case <-token.Done():
		return nil
	}
}
