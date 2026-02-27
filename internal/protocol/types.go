// Package protocol defines wire formats and node state for the gossip protocol (Stage 1).
package protocol

const MessageVersion = 1

// MsgType is the type of a protocol message.
type MsgType string

const (
	MsgHello     MsgType = "HELLO"
	MsgGetPeers  MsgType = "GET_PEERS"
	MsgPeersList MsgType = "PEERS_LIST"
	MsgGossip    MsgType = "GOSSIP"
	MsgPing      MsgType = "PING"
	MsgPong      MsgType = "PONG"
)

// Config holds dynamic protocol parameters.
type Config struct {
	Fanout            int     `json:"fanout"`
	TTL               int     `json:"ttl"`
	PeerLimit         int     `json:"peer_limit"`
	PingIntervalSec   float64 `json:"ping_interval_sec"`
	PeerTimeoutSec    float64 `json:"peer_timeout_sec"`
}

// PeerEntry is one neighbor in the peer list (node_id, addr, last_seen_at).
type PeerEntry struct {
	NodeID     string  `json:"node_id"`
	Addr       string  `json:"addr"`
	LastSeenAt int64   `json:"last_seen_at"` // Unix ms.
	PendingPings int   `json:"pending_pings,omitempty"`
}
