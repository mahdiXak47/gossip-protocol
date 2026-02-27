# Gossip Protocol — Stage 1: Protocol Design

## 1. Overview

- **Goal**: Share information over a P2P network via UDP. Each message is processed at most once per node (deduplication by `msg_id` + seen-set).
- **Transport**: UDP, JSON-encoded messages.
- **Entry points**: One or more **init nodes** (bootstrap nodes). They are not controllers; they only help new nodes join and get initial peers.

### 1.1 Number of init nodes and where they are implemented

- **Number**: The implementation supports **one bootstrap address per node**. You typically run **one** init node (the first node, without `-bootstrap`). You can run more init nodes by starting several nodes without `-bootstrap` on different ports; each would be used by different joining nodes if you passed their address via `-bootstrap`.
- **Implementation**: There is **no separate init-node binary or mode**. The init node is the **same** `node` binary:
  - **Init node**: start with `./node -port 9000` (do **not** pass `-bootstrap`). It listens and handles HELLO, GET_PEERS, PING, PONG, GOSSIP like any other node.
  - **Joining node**: start with `./node -port 8000 -bootstrap 127.0.0.1:9000`. It runs `bootstrapJoin()` once at startup (in `cmd/node/main.go`), which sends HELLO and GET_PEERS to the bootstrap address and waits for PEERS_LIST.
- The node at the bootstrap address does nothing special: when it receives **GET_PEERS** in the main receive loop, it replies with **PEERS_LIST** (its current peer list). So the “init node” is simply a normal node that others contact first; its behavior is the same as every other node (see `case protocol.MsgGetPeers` and `case protocol.MsgPeersList` in the receive loop in `cmd/node/main.go`).

## 2. Node State (what each node stores)

| Field | Format | Purpose |
|-------|--------|--------|
| `node_id` | string (UUID or hash) | Unique ID, independent of IP:PORT. |
| `self_addr` | string `"IP:PORT"` | This node’s listen address. |
| `peers` | list of peer entries | Current neighbors (see below). |
| `seen` | set of `msg_id` (string) | IDs of messages already processed (dedup). |
| `config` | object | Dynamic parameters (see below). |

**Peer entry format** (one per neighbor):

| Field | Format | Purpose |
|-------|--------|--------|
| `node_id` | string | Neighbor’s unique ID. |
| `addr` | string `"IP:PORT"` | Neighbor’s address. |
| `last_seen_at` | int64 (Unix ms) or float | Last time we got a valid response (PONG or any message). |
| `pending_pings` | int (optional) | Count of unanswered pings; used to decide when to mark dead. |

**Config (dynamic parameters):**

| Field | Type | Meaning |
|-------|------|--------|
| `fanout` | int | Number of random neighbors to forward a GOSSIP to. |
| `ttl` | int | Max hops for GOSSIP; decremented each forward; stop when 0. |
| `peer_limit` | int | Max size of `peers` list. |
| `ping_interval_sec` | float | Interval between ping rounds. |
| `peer_timeout_sec` | float | If no response for this long, consider peer dead. |

## 3. Message Format (wire)

All messages share this envelope (JSON over UDP):

```json
{
  "version": 1,
  "msg_id": "uuid-or-hash",
  "msg_type": "HELLO | GET_PEERS | PEERS_LIST | GOSSIP | PING | PONG",
  "sender_id": "node-uuid",
  "sender_addr": "127.0.0.1:8000",
  "timestamp_ms": 1730,
  "ttl": 8,
  "payload": { ... }
}
```

- **`msg_id`**: Unique per message; used for dedup (seen-set).
- **`ttl`**: Used only for GOSSIP; optional or 0 for other types.

Payloads by type:

- **HELLO**: `{"capabilities": ["udp", "json"]}`
- **GET_PEERS**: `{"max_peers": 20}`
- **PEERS_LIST**: `{"peers": [{"node_id":"...", "addr":"127.0.0.1:8001"}, ...]}`
- **GOSSIP**: `{"topic": "news", "data": "...", "origin_id": "node-uuid", "origin_timestamp_ms": 1730}`
- **PING**: `{"ping_id": "uuid-or-counter", "seq": 17}`
- **PONG**: `{"ping_id": "uuid-or-counter", "seq": 17}` (echo back ping_id and seq)

## 4. Bootstrap and Join

1. New node knows **init_node address(es)** (e.g. config or env).
2. New node sends **HELLO** to init_node (introduce self).
3. New node sends **GET_PEERS** (e.g. `max_peers` = config’s `peer_limit` or similar).
4. Init_node (or any node) replies with **PEERS_LIST**.
5. New node adds received peers to `peers` up to `peer_limit` (no duplicates by `node_id`).
6. New node starts **ping/pong** and **GOSSIP** handling like any other node; no further special role for init_node.

## 5. Neighbor Management (liveness)

- **Ping**: Every `ping_interval_sec` seconds, send **PING** to each peer (or a subset if many). Include `ping_id` and `seq` in payload.
- **PONG**: On **PING**, reply with **PONG** with same `ping_id` and `seq`. Update sender’s `last_seen_at` (and clear any pending-ping count for that peer).
- **Timeout**: If a peer does not respond within `peer_timeout_sec` (e.g. after N missed pings or N×ping_interval), mark it **dead**: remove from `peers`. Optional: retry a small number of pings before removal.
- **Eviction**: When at `peer_limit` and a new peer is to be added, evict one (e.g. oldest `last_seen_at` or least recently active) to make room.

## 6. Duplicate Protection (GOSSIP)

- **Per-message ID**: Every message has a unique `msg_id` (envelope).
- **Seen-set**: Each node keeps a set `seen` of `msg_id`s already processed.
- **On receive GOSSIP**:
  1. If `msg_id` is in `seen` → **ignore** (no forward, no log as new).
  2. Else: add `msg_id` to `seen`, (optionally) log application time, then:
     - Decrement `ttl`.
     - If `ttl > 0`: choose **fanout** random neighbors (from current `peers`), send the same GOSSIP (with updated `ttl`) to each; **no retry** if send fails.

So: **TTL** limits propagation depth; **fanout** limits breadth; **seen-set** ensures at most one processing per node.

## 7. Summary Table

| Concern | Design choice |
|--------|----------------|
| Node identity | `node_id` (independent of IP:PORT). |
| Neighbor list | List of `{node_id, addr, last_seen_at}`; max size `peer_limit`. |
| Dedup | `msg_id` + seen-set; process only when `msg_id` not in seen. |
| Join | HELLO → GET_PEERS → PEERS_LIST; fill peers up to `peer_limit`. |
| Liveness | Ping every `ping_interval_sec`; drop peer after `peer_timeout_sec` without response. |
| Eviction | When full, drop oldest or least active peer. |
| GOSSIP spread | Forward to `fanout` random peers; decrement TTL; stop when TTL ≤ 0. |

This is enough to implement Stage 1 (protocol design and data structures). Stages 2+ will add implementation, testing, and metrics (e.g. 5 runs with different seeds, average and standard deviation).
