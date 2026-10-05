package govee

import (
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
	if target == "" {
		target = "239.255.255.250"
	}
	return l.exchange(target, l.ScanPort, LANMessage{Cmd: "scan", Data: map[string]string{"account_topic": "reserve"}}, timeout)
}

// Control returns a matching reply. Mutation commands may not acknowledge receipt.
func (l *LANSocket) Control(ip string, message LANMessage, wait time.Duration) ([]byte, error) {
	replies, err := l.exchange(ip, l.ControlPort, message, wait)
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

func (l *LANSocket) exchange(target string, port int, message LANMessage, wait time.Duration) ([]LANNamedDevice, error) {
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
	if err = sock.SetDeadline(time.Now().Add(wait)); err != nil {
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
