// Package logger provides structured, parseable log lines for analysis (e.g. convergence, overhead).
package logger

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Event fields for parsing. All times in Unix milliseconds.
type Event struct {
	TimeMs  int64             `json:"t_ms"`
	NodeID  string            `json:"node"`
	Event   string            `json:"event"`
	Payload map[string]string `json:"payload,omitempty"`
}

// Logger writes JSON lines to an io.Writer (stdout or file).
type Logger struct {
	w   io.Writer
	mu  sync.Mutex
	buf []byte
}

// New returns a logger writing to w. If w is nil, os.Stdout is used.
func New(w io.Writer) *Logger {
	if w == nil {
		w = os.Stdout
	}
	return &Logger{w: w}
}

// Log writes a structured event as one JSON line.
func (l *Logger) Log(nodeID, event string, payload map[string]string) {
	ev := Event{
		TimeMs:  time.Now().UnixMilli(),
		NodeID:  nodeID,
		Event:   event,
		Payload: payload,
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf, _ = json.Marshal(ev)
	l.buf = append(l.buf, '\n')
	_, _ = l.w.Write(l.buf)
}

// Logf is a convenience for a single key=value payload.
func (l *Logger) Logf(nodeID, event, key, value string) {
	l.Log(nodeID, event, map[string]string{key: value})
}

// LogKV logs event with multiple key-value pairs.
func (l *Logger) LogKV(nodeID, event string, kvs ...string) {
	if len(kvs)%2 != 0 {
		return
	}
	m := make(map[string]string, len(kvs)/2)
	for i := 0; i < len(kvs); i += 2 {
		m[kvs[i]] = kvs[i+1]
	}
	l.Log(nodeID, event, m)
}

// Event names for analysis scripts.
const (
	EvStartup          = "startup"
	EvPeerAdd           = "peer_add"
	EvPeerRemove        = "peer_remove"
	EvGossipReceive     = "gossip_receive"
	EvGossipForward     = "gossip_forward"
	EvGossipOrigin      = "gossip_origin"
	EvDuplicate         = "duplicate"
	EvPingSent          = "ping_sent"
	EvPongReceived      = "pong_received"
	EvPingReceived      = "ping_received"
	EvPongSent          = "pong_sent"
	EvMessageSent       = "message_sent"
	EvInvalidMessage   = "invalid_message"
	EvBootstrapPeers   = "bootstrap_peers"
)
