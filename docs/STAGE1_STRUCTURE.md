 # Gossip protocol — structure and usage

## Project layout

```
gossip protocol/
├── docs/
│   ├── PROTOCOL_DESIGN.md   # Protocol design (node state, messages, bootstrap, dedup)
│   └── STAGE1_STRUCTURE.md  # This file
├── internal/
│   ├── logger/              # Structured JSON-line logging for analysis
│   │   └── logger.go
│   ├── protocol/
│   │   ├── types.go         # MsgType, Config, PeerEntry
│   │   ├── message.go       # Envelope + payload structs
│   │   └── node_state.go    # NodeState, peer list, seen-set
│   └── transport/
│       └── udp.go           # UDP send/receive JSON envelopes
├── cmd/
│   ├── node/                # Single node process (UDP, stdin for gossip)
│   │   └── main.go
│   └── simulate/            # Automated runs (N nodes, multiple seeds)
│       └── main.go
├── go.mod
├── node                     # Built binary
└── simulate                 # Built binary
```

## Running a node

```bash
./node -port 8000 -bootstrap 127.0.0.1:9000 -fanout 3 -ttl 8 -peer-limit 20 \
  -ping-interval 2s -peer-timeout 6s -seed 42
```

Type `gossip <topic> <data>` on stdin to publish a new gossip (e.g. `gossip news hello`).

Optional: `-logfile <path>` writes structured JSON-line logs for analysis.

## Running simulations

```bash
./simulate -nodes 10 -runs 5 -seeds 1,2,3,4,5 -logdir sim_logs -node-bin ./node
```

Supported sizes: N ∈ {10, 20, 50} (any N is valid). Logs are parsed to compute:

- **Convergence time**: ms from gossip origin until 95% of nodes have received it.
- **Message overhead**: total GOSSIP message_sent count for that run.

Summary prints average and standard deviation across runs.

## Structured log events (for parsing)

- `startup`, `peer_add`, `peer_remove`, `bootstrap_peers`
- `gossip_receive`, `gossip_forward`, `gossip_origin`
- `duplicate`, `message_sent`
- `ping_sent`, `pong_received`, `ping_received`, `pong_sent`
- `invalid_message`

Each line is JSON: `{"t_ms":..., "node":"...", "event":"...", "payload":{...}}`.
