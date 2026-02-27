// Package transport provides UDP send/receive for protocol envelopes.
package transport

import (
	"encoding/json"
	"net"

	"github.com/gossip-protocol/internal/protocol"
)

// Send serializes env to JSON and sends it to addr over conn.
func Send(conn *net.UDPConn, addr *net.UDPAddr, env *protocol.Envelope) error {
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = conn.WriteToUDP(b, addr)
	return err
}

// SendToAddr parses addrStr as "IP:port" and sends.
func SendToAddr(conn *net.UDPConn, addrStr string, env *protocol.Envelope) error {
	addr, err := net.ResolveUDPAddr("udp", addrStr)
	if err != nil {
		return err
	}
	return Send(conn, addr, env)
}

// Receive reads one UDP packet and decodes JSON into env.
func Receive(conn *net.UDPConn, env *protocol.Envelope) (*net.UDPAddr, error) {
	buf := make([]byte, 65507)
	n, remote, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(buf[:n], env); err != nil {
		return nil, err
	}
	return remote, nil
}
