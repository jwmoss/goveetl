package govee

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

// LAN uses plaintext JSON: scan on 4001, replies on 4002, control on 4003.
// https://app-h5.govee.com/user-manual/wlan-guide
type LANSocket struct {
	ControlPort int
	ScanPort    int
}

func NewLAN() *LANSocket {
	return &LANSocket{ControlPort: 4003, ScanPort: 4001}
}

type LANNamedDevice struct {
	IP      string
	Message LANMessage
}

type LANMessage struct {
	Cmd  string `json:"cmd"`
	Data any    `json:"data"`
}

type lanPacket struct {
	Message LANMessage `json:"msg"`
}

// Scan accepts a device IP when multicast does not cross the local network.
func (l *LANSocket) Scan(target string, timeout time.Duration) ([]LANNamedDevice, error) {
	return l.ScanContext(context.Background(), target, timeout)
}
func (l *LANSocket) ScanContext(ctx context.Context, target string, timeout time.Duration) ([]LANNamedDevice, error) {
	if target == "" {
		target = "239.255.255.250"
	}
	return l.exchange(ctx, target, l.ScanPort, LANMessage{Cmd: "scan", Data: map[string]string{"account_topic": "reserve"}}, timeout)
}

// Control returns a matching reply. Mutation commands may not acknowledge receipt.
func (l *LANSocket) Control(ip string, message LANMessage, wait time.Duration) ([]byte, error) {
	return l.ControlContext(context.Background(), ip, message, wait)
}
func (l *LANSocket) ControlContext(ctx context.Context, ip string, message LANMessage, wait time.Duration) ([]byte, error) {
	replies, err := l.exchange(ctx, ip, l.ControlPort, message, wait)
	if err != nil {
		return nil, err
	}
	if len(replies) == 0 {
		if message.Cmd == "devStatus" {
			return nil, fmt.Errorf("lan: no status reply from %s", ip)
		}
		return nil, nil
	}
	return json.Marshal(lanPacket{Message: replies[0].Message})
}

func (l *LANSocket) exchange(ctx context.Context, target string, port int, message LANMessage, wait time.Duration) ([]LANNamedDevice, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ip := net.ParseIP(target)
	if ip == nil || ip.To4() == nil {
		return nil, fmt.Errorf("lan: expected an IPv4 address")
	}
	if wait <= 0 {
		return nil, fmt.Errorf("lan: wait must be positive")
	}
	if message.Data == nil {
		message.Data = map[string]any{}
	}
	payload, err := json.Marshal(lanPacket{Message: message})
	if err != nil {
		return nil, err
	}
	var sock *net.UDPConn
	if ip.IsMulticast() {
		sock, err = net.ListenMulticastUDP("udp4", nil, &net.UDPAddr{IP: ip, Port: 4002})
	} else {
		sock, err = net.ListenUDP("udp4", &net.UDPAddr{Port: 4002})
	}
	if err != nil {
		return nil, fmt.Errorf("lan: listen on UDP 4002: %w", err)
	}
	defer sock.Close()
	deadline := time.Now().Add(wait)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	stop := context.AfterFunc(ctx, func() { _ = sock.SetDeadline(time.Now()) })
	defer stop()
	if err = sock.SetDeadline(deadline); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err = sock.WriteToUDP(payload, &net.UDPAddr{IP: ip, Port: port}); err != nil {
		return nil, err
	}
	replies := make([]LANNamedDevice, 0)
	seen := make(map[string]bool)
	buf := make([]byte, 65535)
	for {
		n, addr, err := sock.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if limit, ok := ctx.Deadline(); ok && !time.Now().Before(limit) {
				return nil, context.DeadlineExceeded
			}
			if os.IsTimeout(err) {
				return replies, nil
			}
			return nil, err
		}
		if !ip.IsMulticast() && !addr.IP.Equal(ip) {
			continue
		}
		var packet lanPacket
		if json.Unmarshal(buf[:n], &packet) != nil || packet.Message.Cmd != message.Cmd {
			continue
		}
		if seen[addr.IP.String()] {
			continue
		}
		seen[addr.IP.String()] = true
		replies = append(replies, LANNamedDevice{IP: addr.IP.String(), Message: packet.Message})
		if message.Cmd != "scan" {
			return replies, nil
		}
	}
}
