package govee

import (
	"bytes"
	"crypto/aes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

// LAN API (per Govee's published local API for supported devices):
// discovery = UDP broadcast on port 4002 {"cmd":"scan","data":{"broadcast":true}},
// control/state = UDP port 4001 to the device IP, AES-128-ECB with the
// published 16-byte application key.
//
// ponytail: the LAN key differs by device generation; keep it a config input
// until we verify the default key set against real hardware.

// ErrLANFormat marks undecodable LAN payloads.
var ErrLANFormat = errors.New("lan: malformed payload")

// LANSocket models the LAN control surface.
type LANSocket struct {
	// ControlPort is the UDP port devices listen on (default 4001).
	ControlPort int
	// ScanPort is the UDP port discovery broadcasts target (default 4002).
	ScanPort int
	// Key is the AES-128 key (16 bytes) for the local cipher.
	Key []byte
}

// NewLAN builds a LAN client with default ports.
func NewLAN(key []byte) *LANSocket {
	return &LANSocket{ControlPort: 4001, ScanPort: 4002, Key: key}
}

// LANNamedDevice pairs a discovery reply with its source address.
type LANNamedDevice struct {
	IP      string
	Message LANMessage
}

// LANMessage is the plaintext wire payload.
type LANMessage struct {
	Cmd  string `json:"cmd"`
	Data any    `json:"data,omitempty"`
}

// Scan broadcasts a discovery request for LAN devices.
func (l *LANSocket) Scan(timeout time.Duration) ([]LANNamedDevice, error) {
	sock, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}
	defer sock.Close()
	broadcast, err := net.ResolveUDPAddr("udp4", "255.255.255.255:"+itoa(l.ScanPort))
	if err != nil {
		return nil, err
	}
	payload, err := EncryptLANMessage(l.Key, LANMessage{Cmd: "scan", Data: map[string]bool{"broadcast": true}})
	if err != nil {
		return nil, err
	}
	if _, err := sock.WriteToUDP(payload, broadcast); err != nil {
		return nil, err
	}
	var devices []LANNamedDevice
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 4096)
	if err := sock.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	for time.Now().Before(deadline) {
		n, addr, readErr := sock.ReadFromUDP(buf)
		if readErr != nil {
			break
		}
		plain, decErr := DecryptLANMessage(l.Key, buf[:n])
		if decErr != nil {
			continue // non-LAN skus speak other protocols; skip noise
		}
		var message LANMessage
		if decErr := json.Unmarshal(plain, &message); decErr != nil {
			continue
		}
		devices = append(devices, LANNamedDevice{IP: addr.String(), Message: message})
	}
	return devices, nil
}

// Control sends one encrypted command via UDP and returns any reply.
func (l *LANSocket) Control(ip string, message LANMessage, wait time.Duration) ([]byte, error) {
	addr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(ip, itoa(l.ControlPort)))
	if err != nil {
		return nil, err
	}
	payload, err := EncryptLANMessage(l.Key, message)
	if err != nil {
		return nil, err
	}
	sock, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return nil, err
	}
	defer sock.Close()
	if _, err := sock.Write(payload); err != nil {
		return nil, err
	}
	if wait <= 0 {
		return nil, nil
	}
	if err := sock.SetReadDeadline(time.Now().Add(wait)); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, readErr := sock.Read(buf)
	if readErr != nil {
		if isTimeout(readErr) {
			return nil, nil // fire-and-forget
		}
		return nil, readErr
	}
	plain, err := DecryptLANMessage(l.Key, buf[:n])
	if err != nil {
		return buf[:n], nil
	}
	return plain, nil
}

func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// encryptLAN/decryptLAN: AES-128-ECB with PKCS#7 per the published spec.

// EncryptLANMessage serializes and AES-128-ECB encrypts a LAN payload.
// ponytail: exported for CLI raw use; wrap in per-device helpers if the spec
// grows per-command quirks.
func EncryptLANMessage(key []byte, message LANMessage) ([]byte, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("lan: AES-128 key must be 16 bytes")
	}
	plain, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(plain, aes.BlockSize)
	out := make([]byte, len(padded))
	ecbBlocks(block.Encrypt, out, padded)
	return out, nil
}

// DecryptLANMessage decrypts and parses one LAN reply.
func DecryptLANMessage(key, cipherText []byte) ([]byte, error) {
	if len(key) != 16 || len(cipherText) == 0 || len(cipherText)%aes.BlockSize != 0 {
		return nil, ErrLANFormat
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(cipherText))
	ecbBlocks(block.Decrypt, out, cipherText)
	return pkcs7Unpad(out, aes.BlockSize)
}

// ecbBlocks applies bufferSize-sized ECB roundtrips; Go stdlib has no ECB.
func ecbBlocks(round func(dst, src []byte), dst, src []byte) {
	for i := 0; i < len(src); i += 16 {
		round(dst[i:], src[i:])
	}
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	appended := append(make([]byte, 0, len(data)+pad), data...)
	return append(appended, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, ErrLANFormat
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > blockSize || pad > len(data) {
		return nil, ErrLANFormat
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return nil, ErrLANFormat
		}
	}
	return data[:len(data)-pad], nil
}
