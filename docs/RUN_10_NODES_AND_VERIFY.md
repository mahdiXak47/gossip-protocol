# How to Run 10 Nodes, Publish a Message, and Verify Delivery

This guide walks you through starting a 10-node P2P network, publishing the message **"hey i am working!!!"**, and checking that each node received it using logs.

**Automated run:** From the project root, you can use the script that does all steps and verification:

```bash
./run_10_nodes_verify.sh
```

The script starts 10 nodes, injects `gossip demo hey i am working!!!` from node 9001 after a short delay, waits for propagation, stops the nodes, and prints verification (count of nodes that received the message and RECEIVED/NOT RECEIVED per log file). It exits with code 0 if all 10 nodes received the message, 1 otherwise. The steps below describe the same flow manually.

---

## Prerequisites

- Built binaries: `node` (from `./cmd/node`).
- All commands are run from the project root (the directory containing `node`).

---

## Step 1: Create a directory for logs

```bash
mkdir -p run_logs
```

Each node will write its structured JSON logs to a file in `run_logs/`.

---

## Step 2: Start the bootstrap node (node 0)

**Terminal 1:** Start the **first** node on port **9000**. This node does **not** use `-bootstrap` (it is the entry point for others).

```bash
./node -port 9000 -seed 1 -logfile run_logs/node_9000.log
```

Leave this terminal open. You should see something like:

```
node node-9000 listening on 127.0.0.1:9000 (seed=1). Type 'gossip <topic> <data>' to send.
```

**Important:** All other nodes must use `-bootstrap 127.0.0.1:9000` (the bootstrap node’s address), **not** their own port. If a node bootstraps to itself (e.g. node 9001 with `-bootstrap 127.0.0.1:9001`), it will have no peers and gossip messages will not be sent to anyone.

---

## Step 3: Start the remaining 9 nodes (ports 9001–9009)

Start each of the following in **separate terminals** (Terminals 2–10). Each must use `-bootstrap 127.0.0.1:9000` to join the network (bootstrap = the node from Step 2, not this node’s own port).

**Terminal 2:**
```bash
./node -port 9001 -bootstrap 127.0.0.1:9000 -seed 2 -logfile run_logs/node_9001.log
```

**Terminal 3:**
```bash
./node -port 9002 -bootstrap 127.0.0.1:9000 -seed 3 -logfile run_logs/node_9002.log
```

**Terminal 4:**
```bash
./node -port 9003 -bootstrap 127.0.0.1:9000 -seed 4 -logfile run_logs/node_9003.log
```

**Terminal 5:**
```bash
./node -port 9004 -bootstrap 127.0.0.1:9000 -seed 5 -logfile run_logs/node_9004.log
```

**Terminal 6:**
```bash
./node -port 9005 -bootstrap 127.0.0.1:9000 -seed 6 -logfile run_logs/node_9005.log
```

**Terminal 7:**
```bash
./node -port 9006 -bootstrap 127.0.0.1:9000 -seed 7 -logfile run_logs/node_9006.log
```

**Terminal 8:**
```bash
./node -port 9007 -bootstrap 127.0.0.1:9000 -seed 8 -logfile run_logs/node_9007.log
```

**Terminal 9:**
```bash
./node -port 9008 -bootstrap 127.0.0.1:9000 -seed 9 -logfile run_logs/node_9008.log
```

**Terminal 10:**
```bash
./node -port 9009 -bootstrap 127.0.0.1:9000 -seed 10 -logfile run_logs/node_9009.log
```

Wait a few seconds so that all nodes complete bootstrap and exchange PING/PONG (e.g. 5–10 seconds).

---

## Step 4: Publish the message from one node

Choose **one** node to type the message. For example, use **Terminal 2** (node on port 9001). In that terminal, type:

```text
gossip demo hey i am working!!!
```

Then press Enter.

- **topic:** `demo`
- **data:** `hey i am working!!!`

That node will create a new gossip message with a unique `msg_id` (e.g. `node-9001-1`) and send it to its neighbors. The message will propagate through the network according to TTL and fanout.

You should see on that node’s console something like:

```
gossip received: msg_id=node-9001-1
```

(and possibly similar lines on other nodes’ consoles if they print to stderr/stdout; the authoritative record is in the log files.)

---

## Step 5: Verify each node received the message using logs

All structured events are in `run_logs/node_<port>.log`, one JSON object per line.

### 5.1 Find the message ID (optional but useful)

The node that published the message logs a `gossip_origin` event with `msg_id`. From the node you used (e.g. 9001):

```bash
grep '"event":"gossip_origin"' run_logs/node_9001.log | tail -1
```

Example output:

```json
{"t_ms":1730123456789,"node":"node-9001","event":"gossip_origin","payload":{"msg_id":"node-9001-1","topic":"demo"}}
```

So the `msg_id` for this run is `node-9001-1`. You can use it to filter only this message in the next steps.

### 5.2 Count how many nodes have the message

The **originator** (the node that typed the gossip, e.g. 9001) logs **gossip_origin**, not **gossip_receive**. So “has the message” means either **gossip_origin** (created it) or **gossip_receive** (received it). For a single injected message, you should see **10** nodes total: 1 originator + 9 receivers.

To count nodes that received (only):

```bash
grep '"event":"gossip_receive"' run_logs/node_*.log | wc -l
```

That gives **9** (everyone except the originator). To count **all nodes that have the message** (origin or receive) for a specific msg_id, use the script’s logic or:

```bash
MSG_ID="node-9001-1"   # from gossip_origin in node_9001.log
for f in run_logs/node_*.log; do grep "$MSG_ID" "$f" | grep -q -E '"event":"gossip_receive"|"event":"gossip_origin"' && echo "HAS: $f"; done
```

### 5.3 Verify each node by port (who received the message)

List which node logs contain a `gossip_receive` event:

```bash
for f in run_logs/node_*.log; do
  if grep -q '"event":"gossip_receive"' "$f"; then
    echo "RECEIVED: $f"
  else
    echo "NOT RECEIVED: $f"
  fi
done
```

Expected output (all RECEIVED):

```text
RECEIVED: run_logs/node_9000.log
RECEIVED: run_logs/node_9001.log
RECEIVED: run_logs/node_9002.log
RECEIVED: run_logs/node_9003.log
RECEIVED: run_logs/node_9004.log
RECEIVED: run_logs/node_9005.log
RECEIVED: run_logs/node_9006.log
RECEIVED: run_logs/node_9007.log
RECEIVED: run_logs/node_9008.log
RECEIVED: run_logs/node_9009.log
```

### 5.4 Filter by the specific message ID (optional)

If you saved the `msg_id` (e.g. `node-9001-1`), you can restrict to that message:

```bash
MSG_ID="node-9001-1"
grep "$MSG_ID" run_logs/node_*.log | grep '"event":"gossip_receive"'
```

Or with `jq` (if installed) to parse JSON and filter by event and payload:

```bash
for f in run_logs/node_*.log; do
  if grep -l "gossip_receive" "$f" | xargs -I{} grep "node-9001-1" {} | grep -q "gossip_receive"; then
    echo "RECEIVED (msg node-9001-1): $f"
  fi
done
```

Simpler: just grep for the msg_id in all logs; any line with that id and `gossip_receive` is a node that received this message:

```bash
grep 'node-9001-1' run_logs/node_*.log | grep 'gossip_receive'
```

---

## Step 6: Example log snippets

After publishing **"hey i am working!!!"** (topic `demo`), a node’s log might contain lines like:

**Originator (e.g. node 9001):**
```json
{"t_ms":1730123456789,"node":"node-9001","event":"gossip_origin","payload":{"msg_id":"node-9001-1","topic":"demo"}}
{"t_ms":1730123456790,"node":"node-9001","event":"message_sent","payload":{"type":"GOSSIP","msg_id":"node-9001-1"}}
{"t_ms":1730123456790,"node":"node-9001","event":"gossip_forward","payload":{"msg_id":"node-9001-1","to":"127.0.0.1:9000"}}
...
```

**Another node that received and forwarded it:**
```json
{"t_ms":1730123456795,"node":"node-9000","event":"gossip_receive","payload":{"msg_id":"node-9001-1","from":"127.0.0.1:9001"}}
{"t_ms":1730123456796,"node":"node-9000","event":"gossip_forward","payload":{"msg_id":"node-9001-1","to":"127.0.0.1:9003"}}
...
```

So: **gossip_origin** = this node created the message; **gossip_receive** = this node received it (and processed it once); **gossip_forward** = this node sent it to a neighbor.

---

## Quick reference: commands to test that each node has the message

| What you want | Command |
|---------------|--------|
| Count nodes that received any gossip | `grep '"event":"gossip_receive"' run_logs/node_*.log \| wc -l` (expect 10) |
| List which node files contain a receive | `grep -l '"event":"gossip_receive"' run_logs/node_*.log` |
| RECEIVED / NOT RECEIVED per file | Use the `for f in run_logs/node_*.log; do ...` loop in 5.3 |
| Only the specific message (replace with your msg_id) | `grep 'node-9001-1' run_logs/node_*.log \| grep 'gossip_receive'` |

---

## Stopping the nodes

In each of the 10 terminals, press **Ctrl+C** to stop that node. No special shutdown is required.
