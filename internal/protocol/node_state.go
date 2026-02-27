package protocol

import (
	"math/rand"
	"sync"
	"time"
)

// NodeState holds all persistent state for one node (design Stage 1).
type NodeState struct {
	NodeID   string
	SelfAddr string
	Config   Config
	Peers    []PeerEntry
	Seen     map[string]struct{} // msg_id set for dedup.
	mu       sync.RWMutex
}

// NewNodeState builds initial state; Seen and Peers are empty.
func NewNodeState(nodeID, selfAddr string, cfg Config) *NodeState {
	return &NodeState{
		NodeID:   nodeID,
		SelfAddr: selfAddr,
		Config:   cfg,
		Peers:    nil,
		Seen:     make(map[string]struct{}),
	}
}

// SeenAdd marks msgID as seen. Returns true if it was new (caller should process).
func (n *NodeState) SeenAdd(msgID string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.Seen[msgID]; ok {
		return false
	}
	n.Seen[msgID] = struct{}{}
	return true
}

// SeenContains returns true if msgID was already seen.
func (n *NodeState) SeenContains(msgID string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	_, ok := n.Seen[msgID]
	return ok
}

// PeerByAddr returns the index of a peer with the given addr, or -1.
func (n *NodeState) PeerByAddr(addr string) int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for i := range n.Peers {
		if n.Peers[i].Addr == addr {
			return i
		}
	}
	return -1
}

// PeerByNodeID returns the index of a peer with the given node_id, or -1.
func (n *NodeState) PeerByNodeID(nodeID string) int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for i := range n.Peers {
		if n.Peers[i].NodeID == nodeID {
			return i
		}
	}
	return -1
}

// AddPeer adds a peer if not already present and under peer_limit; evicts oldest by LastSeenAt if full.
// rng is used only for tie-breaking when evicting. Returns true if peer was added.
func (n *NodeState) AddPeer(rng *rand.Rand, entry PeerEntry) bool {
	if entry.NodeID == n.NodeID || entry.Addr == n.SelfAddr {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := range n.Peers {
		if n.Peers[i].NodeID == entry.NodeID || n.Peers[i].Addr == entry.Addr {
			n.Peers[i].LastSeenAt = entry.LastSeenAt
			return false
		}
	}
	if len(n.Peers) >= n.Config.PeerLimit {
		oldest := 0
		for i := 1; i < len(n.Peers); i++ {
			if n.Peers[i].LastSeenAt < n.Peers[oldest].LastSeenAt {
				oldest = i
			}
		}
		n.Peers[oldest] = n.Peers[len(n.Peers)-1]
		n.Peers = n.Peers[:len(n.Peers)-1]
	}
	entry.LastSeenAt = time.Now().UnixMilli()
	n.Peers = append(n.Peers, entry)
	return true
}

// AddPeers adds multiple peers (with eviction when at peer_limit). rng for eviction tie-break.
func (n *NodeState) AddPeers(rng *rand.Rand, entries []PeerEntry) {
	for _, e := range entries {
		n.AddPeer(rng, e)
	}
}

// RemovePeerByAddr removes the peer with the given addr.
func (n *NodeState) RemovePeerByAddr(addr string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := range n.Peers {
		if n.Peers[i].Addr == addr {
			n.Peers[i] = n.Peers[len(n.Peers)-1]
			n.Peers = n.Peers[:len(n.Peers)-1]
			return true
		}
	}
	return false
}

// UpdateLastSeen sets LastSeenAt for the peer at addr to now (Unix ms).
func (n *NodeState) UpdateLastSeen(addr string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	now := time.Now().UnixMilli()
	for i := range n.Peers {
		if n.Peers[i].Addr == addr {
			n.Peers[i].LastSeenAt = now
			n.Peers[i].PendingPings = 0
			return
		}
	}
}

// IncrementPendingPings adds 1 to PendingPings for the peer at addr.
func (n *NodeState) IncrementPendingPings(addr string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := range n.Peers {
		if n.Peers[i].Addr == addr {
			n.Peers[i].PendingPings++
			return
		}
	}
}

// PeersCopy returns a copy of the current peer list.
func (n *NodeState) PeersCopy() []PeerEntry {
	n.mu.RLock()
	defer n.mu.RUnlock()
	out := make([]PeerEntry, len(n.Peers))
	copy(out, n.Peers)
	return out
}

// SelectFanoutRandom returns up to fanout random peer addrs, using rng for deterministic choice.
func (n *NodeState) SelectFanoutRandom(rng *rand.Rand) []string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	peers := n.Peers
	if len(peers) == 0 {
		return nil
	}
	k := n.Config.Fanout
	if k > len(peers) {
		k = len(peers)
	}
	idx := rng.Perm(len(peers))[:k]
	out := make([]string, k)
	for i, j := range idx {
		out[i] = peers[j].Addr
	}
	return out
}

// PeersStale returns addrs of peers that have not been seen since timeout ago (by LastSeenAt).
func (n *NodeState) PeersStale(timeout time.Duration) []string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	cutoff := time.Now().Add(-timeout).UnixMilli()
	var out []string
	for _, p := range n.Peers {
		if p.LastSeenAt < cutoff {
			out = append(out, p.Addr)
		}
	}
	return out
}
