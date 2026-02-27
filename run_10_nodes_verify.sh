#!/usr/bin/env bash
#
# Automated run and verification for 10-node gossip network.
# Based on docs/RUN_10_NODES_AND_VERIFY.md.
#
# Usage: from project root (directory containing ./node):
#   ./run_10_nodes_verify.sh
#
# Prerequisites: ./node must exist (go build -o node ./cmd/node).

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

LOG_DIR="run_logs"
NODE_BIN="./node"
SETTLE_SEC=8
PROPAGATE_SEC=15
GOSSIP_MSG="gossip demo hey i am working!!!"

if [ ! -x "$NODE_BIN" ]; then
  echo "Error: $NODE_BIN not found or not executable. Build with: go build -o node ./cmd/node"
  exit 1
fi

mkdir -p "$LOG_DIR"

echo "=== Starting 10 nodes (bootstrap 9000, peers 9001-9009) ==="

# Bootstrap node (no -bootstrap).
$NODE_BIN -port 9000 -seed 1 -logfile "$LOG_DIR/node_9000.log" >/dev/null 2>&1 &
sleep 2

# Start 9002..9009 first so bootstrap has many peers before 9001 asks for PEERS_LIST.
# (If 9001 connects first, bootstrap only knows 9001 and returns that; 9001 rejects self and gets 0 peers.)
for port in 9002 9003 9004 9005 9006 9007 9008 9009; do
  seed=$((port - 8999))
  $NODE_BIN -port "$port" -bootstrap 127.0.0.1:9000 -seed "$seed" -logfile "$LOG_DIR/node_${port}.log" >/dev/null 2>&1 &
done
sleep 3

# Start 9001 last; it will get 9002..9009 from bootstrap. Inject GOSSIP via stdin after SETTLE_SEC.
( sleep "$SETTLE_SEC"; printf '%s\n' "$GOSSIP_MSG"; sleep "$PROPAGATE_SEC"; sleep 5 ) | \
  $NODE_BIN -port 9001 -bootstrap 127.0.0.1:9000 -seed 2 -logfile "$LOG_DIR/node_9001.log" >/dev/null 2>&1 &

# Disown so shell does not print "Terminated" when we pkill the nodes.
disown -a 2>/dev/null || true

echo "Nodes started. Waiting ${SETTLE_SEC}s + ${PROPAGATE_SEC}s for bootstrap and propagation..."
sleep $((SETTLE_SEC + PROPAGATE_SEC + 6))

echo "=== Stopping nodes ==="
pkill -f "$NODE_BIN -port 90" 2>/dev/null || true
sleep 2
pkill -9 -f "$NODE_BIN -port 90" 2>/dev/null || true
sleep 1

echo ""
echo "=== Verification (see docs/RUN_10_NODES_AND_VERIFY.md) ==="
echo ""

# Get msg_id of the injected message (last gossip_origin from node 9001, topic demo).
MSG_ID=$(grep '"event":"gossip_origin"' "$LOG_DIR/node_9001.log" 2>/dev/null | grep '"topic":"demo"' | tail -1 | grep -o '"msg_id":"[^"]*"' | cut -d'"' -f4)
if [ -z "$MSG_ID" ]; then
  MSG_ID=$(grep '"event":"gossip_origin"' "$LOG_DIR/node_9001.log" 2>/dev/null | tail -1 | grep -o '"msg_id":"[^"]*"' | cut -d'"' -f4)
fi

# Count nodes that have the message: originator has gossip_origin, others have gossip_receive.
HAS_MSG_COUNT=0
for f in "$LOG_DIR"/node_*.log; do
  if grep "$MSG_ID" "$f" 2>/dev/null | grep -q -E '"event":"gossip_receive"|"event":"gossip_origin"'; then
    HAS_MSG_COUNT=$((HAS_MSG_COUNT + 1))
  fi
done

echo "Nodes that have the message (gossip_origin or gossip_receive for msg_id): $HAS_MSG_COUNT / 10"
if [ -n "$MSG_ID" ]; then
  echo "Message ID for this run: $MSG_ID"
fi

# Per-file: has message (origin or receive) or not.
echo ""
echo "Per-node status:"
for f in "$LOG_DIR"/node_*.log; do
  if grep "$MSG_ID" "$f" 2>/dev/null | grep -q -E '"event":"gossip_receive"|"event":"gossip_origin"'; then
    echo "  HAS MESSAGE: $f"
  else
    echo "  NOT RECEIVED: $f"
  fi
done

# Show origin line for reference.
echo ""
ORIGIN_LINE=$(grep '"event":"gossip_origin"' "$LOG_DIR/node_9001.log" 2>/dev/null | tail -1)
if [ -n "$ORIGIN_LINE" ]; then
  echo "Origin (node 9001) last gossip_origin: $ORIGIN_LINE"
fi

echo ""
if [ "$HAS_MSG_COUNT" -eq 10 ]; then
  echo "PASS: All 10 nodes have the message (originator + 9 receivers)."
  exit 0
else
  echo "FAIL: Expected 10 nodes with the message, got $HAS_MSG_COUNT."
  exit 1
fi
