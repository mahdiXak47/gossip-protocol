

## 1. Build the binaries

From the project root (directory containing `go.mod`):

```bash
go build -o node ./cmd/node
go build -o simulate ./cmd/simulate
```

You need both `node` and `simulate` in the same directory (or set `-node-bin` to the path of `node`).

---

## 2. What the simulator outputs

For each **run**, it prints:

- **convergence_ms**: Time (milliseconds) from when the gossip was first sent until **95% of nodes** had received it. (From structured logs: `gossip_origin` time and `gossip_receive` times for that msg_id.)
- **overhead**: Total number of GOSSIP messages sent (count of `message_sent` with type GOSSIP for that msg_id).

At the end it prints a **summary**:

- **convergence_ms: avg=… std=…** — mean and sample standard deviation of convergence time over all runs.
- **message_overhead: avg=… std=…** — mean and sample standard deviation of message overhead over all runs.

---

## 3. Basic usage

**Example: 10 nodes, 5 runs (default seeds 1,2,3,4,5)**

```bash
./simulate -nodes 10 -runs 5
```

**Example: 20 nodes, 5 runs, explicit seeds**

```bash
./simulate -nodes 20 -runs 5 -seeds 1,2,3,4,5
```

**Example: 50 nodes, 5 runs, custom timing**

```bash
./simulate -nodes 50 -runs 5 -settle 5s -run-dur 30s
```

**All flags**

| Flag         | Default  | Meaning |
|-------------|----------|--------|
| `-nodes`    | 10       | Number of nodes (e.g. 10, 20, 50). |
| `-runs`     | 5        | Number of runs (each with a different seed). |
| `-seeds`    | (empty)  | Comma-separated seeds; if empty, uses 1,2,…,runs. |
| `-base-port`| 9000     | First node port (nodes use base-port, base-port+1, …). |
| `-settle`   | 5s       | Wait after starting nodes before injecting gossip. |
| `-run-dur`  | 30s      | Time to let gossip propagate after injection. |
| `-logdir`   | sim_logs | Directory for per-run, per-node log files. |
| `-node-bin` | ./node   | Path to the node binary. |

---

## 4. Getting mean and standard deviation for your report

Run the simulator with the **network sizes** and **number of runs** you want (e.g. N ∈ {10, 20, 50} and at least 5 runs):

```bash
# 10 nodes, 5 runs
./simulate -nodes 10 -runs 5 -logdir sim_logs_10

# 20 nodes, 5 runs (use a different logdir so logs don’t mix)
./simulate -nodes 20 -runs 5 -logdir sim_logs_20

# 50 nodes, 5 runs
./simulate -nodes 50 -runs 5 -logdir sim_logs_50
```

**Example output:**

```text
run 0: convergence_ms=120 overhead=45
run 1: convergence_ms=98 overhead=42
run 2: convergence_ms=115 overhead=48
run 3: convergence_ms=105 overhead=44
run 4: convergence_ms=112 overhead=46
--- summary ---
convergence_ms: avg=110.00 std=8.94
message_overhead: avg=45.00 std=2.45
```

Use the **summary** section for your report:

- **Mean (avg)** and **standard deviation (std)** for convergence time (ms).
- **Mean (avg)** and **standard deviation (std)** for message overhead.

You can run each of the three commands above and fill a table like:

| N (nodes) | Runs | Convergence (ms): mean ± std | Overhead: mean ± std |
|-----------|------|------------------------------|----------------------|
| 10        | 5    | (paste from summary)         | (paste from summary) |
| 20        | 5    | …                            | …                    |
| 50        | 5    | …                            | …                    |

---

## 5. Saving output to a file

To keep the numbers for your report:

```bash
./simulate -nodes 10 -runs 5 -logdir sim_logs_10 2>&1 | tee results_10nodes.txt
```

Then open `results_10nodes.txt` and copy the per-run lines and the summary (mean and standard deviation).

---

## 6. Notes

- **convergence_ms** is the time from the first `gossip_origin` (topic=sim) until the moment when 95% of nodes have logged `gossip_receive` for that message. If in some run the message doesn’t reach 95% of nodes (e.g. logs incomplete or timeout), that run may show `convergence_ms=-1` and is not included in the average.
- **overhead** is the total number of GOSSIP sends for that run’s message (each forward counts).
- Logs are written under `-logdir` (e.g. `sim_logs/run_0/node_0.log`, …). You can inspect or post-process these JSON-lines files for more detailed analysis.
- If the node binary is not in the current directory, set it explicitly:  
  `./simulate -nodes 10 -runs 5 -node-bin /path/to/node`
