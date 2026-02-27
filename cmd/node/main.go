// Command node runs a single gossip protocol node (UDP, JSON).
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gossip-protocol/internal/logger"
	"github.com/gossip-protocol/internal/protocol"
	"github.com/gossip-protocol/internal/transport"
)

func main() {
	port := flag.Int("port", 8000, "UDP listen port")
	bootstrap := flag.String("bootstrap", "", "Bootstrap node address (e.g. 127.0.0.1:9000)")
	fanout := flag.Int("fanout", 3, "Number of neighbors to forward gossip to")
	ttl := flag.Int("ttl", 8, "Max gossip propagation depth")
	peerLimit := flag.Int("peer-limit", 20, "Max number of neighbors")
	pingInterval := flag.Duration("ping-interval", 2*time.Second, "Interval between ping rounds")
	peerTimeout := flag.Duration("peer-timeout", 6*time.Second, "Timeout before considering a peer dead")
	seed := flag.Int64("seed", 0, "Random seed for deterministic behavior")
	logFile := flag.String("logfile", "", "Write structured logs to this file (default stdout)")
	flag.Parse()

	var logWriter *os.File
	if *logFile != "" {
		var err error
		logWriter, err = os.Create(*logFile)
		if err != nil {
			log.Fatalf("logfile: %v", err)
		}
		defer logWriter.Close()
	}
	sl := logger.New(logWriter)

	selfAddr := fmt.Sprintf("127.0.0.1:%d", *port)
	nodeID := fmt.Sprintf("node-%d", *port)
	rng := rand.New(rand.NewSource(*seed))
	cfg := protocol.Config{
		Fanout:          *fanout,
		TTL:             *ttl,
		PeerLimit:       *peerLimit,
		PingIntervalSec: pingInterval.Seconds(),
		PeerTimeoutSec:  peerTimeout.Seconds(),
	}
	state := protocol.NewNodeState(nodeID, selfAddr, cfg)

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: *port})
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer conn.Close()

	sl.LogKV(nodeID, logger.EvStartup, "port", fmt.Sprint(*port), "addr", selfAddr, "seed", fmt.Sprint(*seed))
	log.Printf("node %s listening on %s (seed=%d). Type 'gossip <topic> <data>' to send.", nodeID, selfAddr, *seed)

	var bootstrapAddrs []string
	if *bootstrap != "" {
		bootstrapAddrs = append(bootstrapAddrs, *bootstrap)
	}
	if len(bootstrapAddrs) > 0 {
		bootstrapJoin(conn, state, rng, nodeID, selfAddr, bootstrapAddrs[0], &cfg, sl)
	}

	go runStdinGossip(conn, state, rng, nodeID, selfAddr, &cfg, sl)
	go runPingLoop(conn, state, rng, nodeID, selfAddr, *pingInterval, sl)
	go runDeadPeerPrune(state, *peerTimeout, nodeID, sl)

	runReceiveLoop(conn, state, rng, nodeID, selfAddr, &cfg, sl)
}

var msgIDCounter uint64

func nextMsgID(nodeID string) string {
	c := atomic.AddUint64(&msgIDCounter, 1)
	return fmt.Sprintf("%s-%d", nodeID, c)
}

func bootstrapJoin(conn *net.UDPConn, state *protocol.NodeState, rng *rand.Rand, nodeID, selfAddr, bootstrapAddr string, cfg *protocol.Config, sl *logger.Logger) {
	helloPayload, _ := json.Marshal(protocol.HelloPayload{Capabilities: []string{"udp", "json"}})
	env := &protocol.Envelope{
		Version:     protocol.MessageVersion,
		MsgID:       nextMsgID(nodeID),
		MsgType:     protocol.MsgHello,
		SenderID:    nodeID,
		SenderAddr:  selfAddr,
		TimestampMs: time.Now().UnixMilli(),
		TTL:         0,
		Payload:     helloPayload,
	}
	if err := transport.SendToAddr(conn, bootstrapAddr, env); err != nil {
		log.Printf("bootstrap HELLO send: %v", err)
		return
	}

	getPayload, _ := json.Marshal(protocol.GetPeersPayload{MaxPeers: cfg.PeerLimit})
	env.MsgID = nextMsgID(nodeID)
	env.MsgType = protocol.MsgGetPeers
	env.Payload = getPayload
	if err := transport.SendToAddr(conn, bootstrapAddr, env); err != nil {
		log.Printf("bootstrap GET_PEERS send: %v", err)
		return
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetReadDeadline(time.Time{})
	var list protocol.PeersListPayload
	for i := 0; i < 5; i++ {
		var e protocol.Envelope
		_, err := transport.Receive(conn, &e)
		if err != nil {
			break
		}
		if e.MsgType == protocol.MsgPeersList {
			if err := json.Unmarshal(e.Payload, &list); err != nil {
				continue
			}
			before := len(state.PeersCopy())
			state.AddPeers(rng, list.Peers)
			// So the bootstrap node can receive/forward gossip, add it as a peer (it is not in PEERS_LIST).
			state.AddPeer(rng, protocol.PeerEntry{NodeID: "bootstrap", Addr: bootstrapAddr, LastSeenAt: time.Now().UnixMilli()})
			after := len(state.PeersCopy())
			sl.LogKV(nodeID, logger.EvBootstrapPeers, "from", bootstrapAddr, "count", fmt.Sprint(after-before))
			log.Printf("bootstrap: got %d peers from %s", len(list.Peers), bootstrapAddr)
			break
		}
	}
}

func runStdinGossip(conn *net.UDPConn, state *protocol.NodeState, rng *rand.Rand, nodeID, selfAddr string, cfg *protocol.Config, sl *logger.Logger) {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 2 || !strings.EqualFold(parts[0], "gossip") {
			continue
		}
		topic := parts[1]
		data := ""
		if len(parts) == 3 {
			data = parts[2]
		}
		emitGossip(conn, state, rng, nodeID, selfAddr, topic, data, cfg, sl)
	}
}

func emitGossip(conn *net.UDPConn, state *protocol.NodeState, rng *rand.Rand, nodeID, selfAddr, topic, data string, cfg *protocol.Config, sl *logger.Logger) {
	msgID := nextMsgID(nodeID)
	state.SeenAdd(msgID)
	payload, _ := json.Marshal(protocol.GossipPayload{
		Topic:             topic,
		Data:              data,
		OriginID:          nodeID,
		OriginTimestampMs: time.Now().UnixMilli(),
	})
	env := &protocol.Envelope{
		Version:     protocol.MessageVersion,
		MsgID:       msgID,
		MsgType:     protocol.MsgGossip,
		SenderID:    nodeID,
		SenderAddr:  selfAddr,
		TimestampMs: time.Now().UnixMilli(),
		TTL:         cfg.TTL,
		Payload:     payload,
	}
	sl.LogKV(nodeID, logger.EvGossipOrigin, "msg_id", msgID, "topic", topic)
	sl.LogKV(nodeID, logger.EvMessageSent, "type", "GOSSIP", "msg_id", msgID)
	for _, addr := range state.SelectFanoutRandom(rng) {
		if err := transport.SendToAddr(conn, addr, env); err != nil {
			continue
		}
		sl.LogKV(nodeID, logger.EvGossipForward, "msg_id", msgID, "to", addr)
	}
}

func runPingLoop(conn *net.UDPConn, state *protocol.NodeState, rng *rand.Rand, nodeID, selfAddr string, interval time.Duration, sl *logger.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	seq := int64(0)
	for range ticker.C {
		peers := state.PeersCopy()
		seq++
		pingID := fmt.Sprintf("%s-ping-%d", nodeID, seq)
		payload, _ := json.Marshal(protocol.PingPayload{PingID: pingID, Seq: seq})
		for _, p := range peers {
			env := &protocol.Envelope{
				Version:     protocol.MessageVersion,
				MsgID:       nextMsgID(nodeID),
				MsgType:     protocol.MsgPing,
				SenderID:    nodeID,
				SenderAddr:  selfAddr,
				TimestampMs: time.Now().UnixMilli(),
				TTL:         0,
				Payload:     payload,
			}
			_ = transport.SendToAddr(conn, p.Addr, env)
			state.IncrementPendingPings(p.Addr)
			sl.LogKV(nodeID, logger.EvPingSent, "to", p.Addr, "seq", fmt.Sprint(seq))
		}
	}
}

func runDeadPeerPrune(state *protocol.NodeState, timeout time.Duration, nodeID string, sl *logger.Logger) {
	ticker := time.NewTicker(timeout / 2)
	defer ticker.Stop()
	for range ticker.C {
		stale := state.PeersStale(timeout)
		for _, addr := range stale {
			if state.RemovePeerByAddr(addr) {
				sl.LogKV(nodeID, logger.EvPeerRemove, "addr", addr, "reason", "timeout")
				log.Printf("peer %s timed out, removed", addr)
			}
		}
	}
}

func runReceiveLoop(conn *net.UDPConn, state *protocol.NodeState, rng *rand.Rand, nodeID, selfAddr string, cfg *protocol.Config, sl *logger.Logger) {
	for {
		var env protocol.Envelope
		remote, err := transport.Receive(conn, &env)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			log.Printf("receive: %v", err)
			continue
		}
		remoteStr := remote.String()
		if host, port, err := net.SplitHostPort(remoteStr); err == nil {
			remoteStr = host + ":" + port
		}

		// Invalid or unknown msg_type (e.g. bad JSON leaves env zero) — do not crash.
		if env.MsgType == "" {
			sl.LogKV(nodeID, logger.EvInvalidMessage, "from", remoteStr)
			continue
		}

		switch env.MsgType {
		case protocol.MsgHello:
			added := state.AddPeer(rng, protocol.PeerEntry{NodeID: env.SenderID, Addr: env.SenderAddr, LastSeenAt: time.Now().UnixMilli()})
			if added {
				sl.LogKV(nodeID, logger.EvPeerAdd, "addr", env.SenderAddr, "node_id", env.SenderID)
			}
		case protocol.MsgGetPeers:
			state.UpdateLastSeen(remoteStr)
			peers := state.PeersCopy()
			pl, _ := json.Marshal(protocol.PeersListPayload{Peers: peers})
			resp := &protocol.Envelope{
				Version:     protocol.MessageVersion,
				MsgID:       nextMsgID(nodeID),
				MsgType:     protocol.MsgPeersList,
				SenderID:    nodeID,
				SenderAddr:  selfAddr,
				TimestampMs: time.Now().UnixMilli(),
				TTL:         0,
				Payload:     pl,
			}
			_ = transport.Send(conn, remote, resp)
		case protocol.MsgPeersList:
			state.UpdateLastSeen(remoteStr)
			var list protocol.PeersListPayload
			if err := json.Unmarshal(env.Payload, &list); err != nil {
				continue
			}
			state.AddPeers(rng, list.Peers)
		case protocol.MsgPing:
			state.UpdateLastSeen(remoteStr)
			sl.LogKV(nodeID, logger.EvPingReceived, "from", remoteStr)
			var p protocol.PingPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				continue
			}
			pongPayload, _ := json.Marshal(protocol.PongPayload{PingID: p.PingID, Seq: p.Seq})
			resp := &protocol.Envelope{
				Version:     protocol.MessageVersion,
				MsgID:       nextMsgID(nodeID),
				MsgType:     protocol.MsgPong,
				SenderID:    nodeID,
				SenderAddr:  selfAddr,
				TimestampMs: time.Now().UnixMilli(),
				TTL:         0,
				Payload:     pongPayload,
			}
			_ = transport.Send(conn, remote, resp)
			sl.LogKV(nodeID, logger.EvPongSent, "to", remoteStr)
		case protocol.MsgPong:
			state.UpdateLastSeen(remoteStr)
			sl.LogKV(nodeID, logger.EvPongReceived, "from", remoteStr)
		case protocol.MsgGossip:
			if state.SeenContains(env.MsgID) {
				sl.LogKV(nodeID, logger.EvDuplicate, "msg_id", env.MsgID)
				continue
			}
			state.SeenAdd(env.MsgID)
			sl.LogKV(nodeID, logger.EvGossipReceive, "msg_id", env.MsgID, "from", remoteStr)
			log.Printf("gossip received: msg_id=%s", env.MsgID)
			ttl := env.TTL - 1
			if ttl <= 0 {
				continue
			}
			env.TTL = ttl
			for _, addr := range state.SelectFanoutRandom(rng) {
				if addr == remoteStr {
					continue
				}
				if err := transport.SendToAddr(conn, addr, &env); err != nil {
					continue
				}
				sl.LogKV(nodeID, logger.EvGossipForward, "msg_id", env.MsgID, "to", addr)
				sl.LogKV(nodeID, logger.EvMessageSent, "type", "GOSSIP", "msg_id", env.MsgID)
			}
		default:
			// Ignore unknown types.
		}
	}
}
