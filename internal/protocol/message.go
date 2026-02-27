package protocol

import "encoding/json"

// Envelope is the common message envelope over UDP (JSON).
type Envelope struct {
	Version   int       `json:"version"`
	MsgID     string    `json:"msg_id"`
	MsgType   MsgType   `json:"msg_type"`
	SenderID  string    `json:"sender_id"`
	SenderAddr string   `json:"sender_addr"`
	TimestampMs int64   `json:"timestamp_ms"`
	TTL       int       `json:"ttl"`
	Payload   json.RawMessage `json:"payload"`
}

// Payloads by message type.

type HelloPayload struct {
	Capabilities []string `json:"capabilities"`
}

type GetPeersPayload struct {
	MaxPeers int `json:"max_peers"`
}

type PeersListPayload struct {
	Peers []PeerEntry `json:"peers"`
}

type GossipPayload struct {
	Topic             string `json:"topic"`
	Data              string `json:"data"`
	OriginID          string `json:"origin_id"`
	OriginTimestampMs int64  `json:"origin_timestamp_ms"`
}

type PingPayload struct {
	PingID string `json:"ping_id"`
	Seq    int64  `json:"seq"`
}

type PongPayload struct {
	PingID string `json:"ping_id"`
	Seq    int64  `json:"seq"`
}
