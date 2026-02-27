# Gossip Protocol — User Guide

This document explains the concepts behind the application and how to use it to build and operate a peer-to-peer (P2P) network.

---

## 1. Concepts

### 1.1 Peer-to-peer (P2P) network

In a P2P network there is **no central server**. Each participant is a **node** that can talk directly to other nodes. Nodes discover each other and exchange information without a single point of control or failure.

This application implements a **gossip-based** P2P system: nodes share information by “gossiping”—sending updates to a subset of neighbors, who then forward to their neighbors, and so on.

### 1.2 Gossip protocol (push model)

- **Push gossip**: When a node has new information, it **pushes** it to some neighbors. Those neighbors push to their neighbors, and the message spreads through the network.
- **No pull by default**: Nodes do not periodically ask “what’s new?”; they only forward what they receive. (Future extensions can add pull, e.g. IHAVE/IWANT.)
- **Random selection**: Each node forwards a message to a **random** subset of its neighbors (size = **fanout**). This avoids everyone sending to the same nodes and helps the message reach the whole network.

### 1.3 Message deduplication (seen-set and msg_id)

- Every message has a unique **msg_id** (e.g. `node-8000-1`).
- Each node keeps a **seen-set**: the set of `msg_id`s it has already processed.
- When a node receives a message:
  - If **msg_id is in the seen-set** → the message is **ignored** (duplicate).
  - Otherwise → the node **adds msg_id to the seen-set**, processes the message (e.g. logs it, applies it), and may forward it.

This guarantees **at most one processing per node per message**, even if the same message arrives multiple times from different neighbors.

### 1.4 TTL (time-to-live)

- **TTL** is the maximum number of **hops** a gossip message can travel.
- When a node forwards a GOSSIP message, it **decrements TTL by 1**.
- If **TTL ≤ 0** after decrement, the node **does not forward** (stops propagation).
- TTL prevents a message from circulating forever and limits how far it spreads.

### 1.5 Fanout

- **Fanout** is the number of **random neighbors** a node chooses when it forwards a GOSSIP message.
- Example: fanout = 3 → each node sends the message to 3 randomly chosen peers (if it has at least 3).
- Higher fanout → faster spread, more messages. Lower fanout → slower spread, fewer messages.

### 1.6 Node state (what each node stores)

| Concept        | Meaning |
|----------------|--------|
| **node_id**    | Unique identifier for this node (e.g. `node-8000`). Independent of IP:port. |
| **self_addr**  | This node’s address: `IP:PORT` (e.g. `127.0.0.1:8000`). |
| **Peer list**  | Current neighbors: for each peer, we store `node_id`, `addr`, and `last_seen_at`. |
| **Seen-set**   | Set of `msg_id`s already processed (for deduplication). |
| **Config**     | Parameters: fanout, ttl, peer_limit, ping_interval, peer_timeout. |

The peer list is **bounded** by **peer_limit**. When the list is full and a new peer is added, the node **evicts** the one with the oldest `last_seen_at`.

### 1.7 Bootstrap (joining the network)

A new node does not know anyone. It needs at least one **bootstrap** (entry) address—a node that is already in the network.

**Join flow:**

1. New node sends **HELLO** to the bootstrap address (introduces itself).
2. New node sends **GET_PEERS** (requests a list of known peers).
3. Bootstrap node (or any node that receives GET_PEERS) replies with **PEERS_LIST**.
4. New node adds the received peers to its peer list (up to **peer_limit**).
5. From then on, the new node behaves like any other: it pings its peers, responds to pings, and forwards gossip. The bootstrap node is **not** a central controller—it only helped with the first contact.

### 1.8 Neighbor liveness (PING / PONG)

- Nodes need to know which peers are still alive.
- **PING**: Periodically (every **ping_interval**), each node sends a PING to its peers.
- **PONG**: When a node receives a PING, it replies with a PONG (same ping_id and seq).
- **last_seen_at**: Updated whenever we receive a PONG or any message from that peer.
- **Dead peer**: If a peer does not respond for **peer_timeout**, the node considers it dead and **removes it** from the peer list.

### 1.9 Protocol message types

| Type       | Who sends        | Purpose |
|------------|------------------|--------|
| **HELLO**  | New node         | Introduce self to bootstrap/peer. |
| **GET_PEERS** | New node / any | Request list of known peers. |
| **PEERS_LIST** | Any node      | Response: list of peers (node_id, addr). |
| **GOSSIP** | Any node         | Disseminate information; forwarded by recipients. |
| **PING**   | Any node         | Liveness probe. |
| **PONG**   | Any node         | Response to PING. |

All messages are sent as **JSON over UDP** and share a common **envelope** (version, msg_id, msg_type, sender_id, sender_addr, timestamp_ms, ttl, payload).

---

## 2. Building the application

Prerequisites: Go 1.21+.

```bash
cd "gossip protocol"
go build -o node ./cmd/node
go build -o simulate ./cmd/simulate
```

You should get two binaries: **node** (single P2P node) and **simulate** (automated experiments).

---

## 3. How to build a peer-to-peer network

### 3.1 Start the first node (bootstrap / init node)

This node does **not** use `-bootstrap`; it is the entry point others will use.

```bash
./node -port 9000 -seed 1
```

It listens on `127.0.0.1:9000`. Leave it running.

### 3.2 Start more nodes that join via bootstrap

In **other terminals**, start nodes that point to the first node:

```bash
./node -port 8000 -bootstrap 127.0.0.1:9000 -seed 2
./node -port 8001 -bootstrap 127.0.0.1:9000 -seed 3
./node -port 8002 -bootstrap 127.0.0.1:9000 -seed 4
```

Each new node will:

1. Send HELLO and GET_PEERS to `127.0.0.1:9000`.
2. Receive PEERS_LIST and fill its peer list (up to peer_limit).
3. Start ping/pong and gossip like any other node.

You now have a small P2P network: all nodes know each other (or a subset), and there is no central controller.

### 3.3 Running many nodes on one machine (e.g. 10–50)

Use different ports and the same bootstrap:

```bash
# Terminal 1: bootstrap
./node -port 9000 -seed 1

# Terminals 2–11 (or run in background):
./node -port 9001 -bootstrap 127.0.0.1:9000 -seed 2
./node -port 9002 -bootstrap 127.0.0.1:9000 -seed 3
# ... up to port 9050 for 51 nodes
```

For fully automated runs with many nodes, use **simulate** (see below).

---

## 4. Node usage and functionalities

### 4.1 Command-line flags (node)

| Flag              | Default   | Meaning |
|-------------------|-----------|--------|
| `-port`           | 8000      | UDP listen port. |
| `-bootstrap`      | (none)    | Bootstrap node address, e.g. `127.0.0.1:9000`. |
| `-fanout`         | 3         | Number of random neighbors to forward gossip to. |
| `-ttl`            | 8         | Max hops for gossip propagation. |
| `-peer-limit`     | 20        | Max number of peers. |
| `-ping-interval`  | 2s        | Interval between ping rounds. |
| `-peer-timeout`   | 6s        | Time without response before a peer is removed. |
| `-seed`           | 0         | Random seed (for deterministic behavior). |
| `-logfile`        | (none)    | Write structured JSON logs to this file. |

Example:

```bash
./node -port 8000 -bootstrap 127.0.0.1:9000 -fanout 3 -ttl 8 -peer-limit 20 \
  -ping-interval 2s -peer-timeout 6s -seed 42
```

### 4.2 Sending gossip (user input)

While the node is running, you can **create and spread a new gossip message** by typing on **stdin**:

```text
gossip <topic> <data>
```

Examples:

- `gossip news Hello world`  — topic `news`, data `Hello world`
- `gossip alerts node-8000 is up`  — topic `alerts`, data `node-8000 is up`
- `gossip sim run-0`  — used by the simulator (topic `sim`)

What happens:

1. The node generates a new **msg_id** and adds it to its seen-set.
2. It builds a GOSSIP message with topic and data.
3. It sends the message to **fanout** random peers (if it has any).
4. Those peers apply dedup (seen-set), then forward to their fanout, and so on, until TTL reaches 0.

So: **to disseminate information in the P2P network, type `gossip <topic> <data>` on any node’s stdin.**

### 4.3 Writing logs to a file

To analyze behavior or run scripts on logs, write structured logs to a file:

```bash
./node -port 8000 -bootstrap 127.0.0.1:9000 -logfile node_8000.log
```

Each log line is one JSON object with: `t_ms` (timestamp), `node`, `event`, and optional `payload`. Events include: `startup`, `peer_add`, `peer_remove`, `gossip_receive`, `gossip_forward`, `gossip_origin`, `duplicate`, `ping_sent`, `pong_received`, `message_sent`, `invalid_message`, etc.

---

## 5. Automated simulations

The **simulate** binary starts many nodes, injects one gossip message, then parses logs to compute **convergence time** and **message overhead**.

### 5.1 What the simulator does

1. For each **run** (e.g. 5 runs):
   - Starts **N** node processes (ports base_port, base_port+1, …).
   - Node 0 is the bootstrap (no `-bootstrap`); nodes 1..N-1 use `-bootstrap 127.0.0.1:<base_port>`.
   - Waits **settle** time so peers can discover each other.
   - Injects one gossip by writing `gossip sim run-<r>` to **node 1’s** stdin (or node 0 if N=1).
   - Waits **run-dur** for propagation.
   - Stops all nodes and parses the log files.
2. From logs it computes:
   - **Convergence time**: time from the first `gossip_origin` (topic=sim) until **95% of nodes** have logged `gossip_receive` for that msg_id (in milliseconds).
   - **Message overhead**: total number of `message_sent` events with type GOSSIP for that msg_id.
3. Prints per-run results and **average ± standard deviation** over runs.

### 5.2 Simulator flags

| Flag          | Default | Meaning |
|---------------|---------|--------|
| `-nodes`      | 10      | Number of nodes (e.g. 10, 20, 50). |
| `-runs`       | 5       | Number of runs. |
| `-seeds`      | (none)  | Comma-separated seeds; if empty, seeds 1..runs are used. |
| `-base-port`  | 9000    | First node’s port. |
| `-settle`     | 5s      | Wait time after startup before injecting gossip. |
| `-run-dur`    | 30s     | Time to let gossip propagate after injection. |
| `-logdir`     | sim_logs | Directory for per-run, per-node log files. |
| `-node-bin`   | ./node  | Path to the node binary. |

### 5.3 How to run simulations

Example: 10 nodes, 5 runs, seeds 1–5, logs in `sim_logs`:

```bash
./simulate -nodes 10 -runs 5 -seeds 1,2,3,4,5 -logdir sim_logs -node-bin ./node
```

Example: 20 nodes, 3 runs, default seeds and timing:

```bash
./simulate -nodes 20 -runs 3
```

Example: 50 nodes, 5 runs, shorter settle and run for a quick test:

```bash
./simulate -nodes 50 -runs 5 -settle 3s -run-dur 15s
```

Output will look like:

```text
run 0: convergence_ms=120 overhead=45
run 1: convergence_ms=98 overhead=42
...
--- summary ---
convergence_ms: avg=105.20 std=15.30
message_overhead: avg=44.00 std=2.50
```

---

## 6. Summary: doing everything with this app

| Goal | How |
|------|-----|
| **Build a P2P network** | Start one node without `-bootstrap`, then start others with `-bootstrap <first_node_addr>`. |
| **Spread information** | On any running node, type `gossip <topic> <data>` on stdin. |
| **Tune propagation** | Use `-fanout` and `-ttl`; use `-peer-limit` to cap neighbors. |
| **Tune liveness** | Use `-ping-interval` and `-peer-timeout`. |
| **Reproducible runs** | Use `-seed` on nodes and `-seeds` in simulate. |
| **Log for analysis** | Use `-logfile <path>` on nodes; simulate writes to `-logdir`. |
| **Measure convergence and overhead** | Run `./simulate -nodes N -runs R` and read the printed summary. |

For protocol details (message formats, state, bootstrap flow), see **PROTOCOL_DESIGN.md**. For project layout and code organization, see **STAGE1_STRUCTURE.md**.
