// Command simulate runs automated gossip experiments: N nodes, multiple runs with different seeds.
// Logs are parsed to compute convergence time (95% nodes received) and message overhead.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gossip-protocol/internal/logger"
)

func main() {
	nodes := flag.Int("nodes", 10, "Number of nodes (e.g. 10, 20, 50)")
	runs := flag.Int("runs", 5, "Number of runs per experiment")
	seedsStr := flag.String("seeds", "", "Comma-separated seeds (e.g. 1,2,3,4,5). If empty, uses 1..runs")
	basePort := flag.Int("base-port", 9000, "First node port; nodes use base-port, base-port+1, ...")
	settleDur := flag.Duration("settle", 5*time.Second, "Time to wait after startup before injecting gossip")
	runDur := flag.Duration("run-dur", 30*time.Second, "Time to run after injecting gossip")
	logDir := flag.String("logdir", "sim_logs", "Directory for per-run log files")
	nodeBin := flag.String("node-bin", "./node", "Path to node binary")
	flag.Parse()

	seeds := parseSeeds(*seedsStr, *runs)
	if len(seeds) < *runs {
		for i := len(seeds); i < *runs; i++ {
			seeds = append(seeds, i+1)
		}
	}

	// Ensure node binary exists.
	if _, err := os.Stat(*nodeBin); err != nil {
		fmt.Fprintf(os.Stderr, "node binary not found at %s: %v\n", *nodeBin, err)
		os.Exit(1)
	}

	absLogDir, _ := filepath.Abs(*logDir)
	_ = os.MkdirAll(absLogDir, 0755)

	var convMs []float64
	var overhead []int

	for r := 0; r < *runs; r++ {
		runDir := filepath.Join(absLogDir, fmt.Sprintf("run_%d", r))
		_ = os.MkdirAll(runDir, 0755)

		cmds, stdinPipe := startNodes(*nodeBin, *nodes, *basePort, runDir, seeds[r])
		if len(cmds) == 0 {
			fmt.Fprintf(os.Stderr, "run %d: failed to start nodes\n", r)
			continue
		}

		time.Sleep(*settleDur)
		// Inject one GOSSIP from node 1 (index 1); topic=sim so we can find msg_id in logs.
		_, _ = stdinPipe.WriteString("gossip sim run-" + strconv.Itoa(r) + "\n")
		_ = stdinPipe.Flush()

		time.Sleep(*runDur)
		for _, c := range cmds {
			if c.Process != nil {
				_ = c.Process.Kill()
			}
		}
		time.Sleep(100 * time.Millisecond)

		cms, ov := parseRunLogs(runDir, *nodes)
		if cms >= 0 {
			convMs = append(convMs, float64(cms))
		}
		overhead = append(overhead, ov)
		fmt.Printf("run %d: convergence_ms=%v overhead=%d\n", r, cms, ov)
	}

	fmt.Println("--- summary ---")
	if len(convMs) > 0 {
		avg, std := meanStd(convMs)
		fmt.Printf("convergence_ms: avg=%.2f std=%.2f\n", avg, std)
	}
	if len(overhead) > 0 {
		ovF := make([]float64, len(overhead))
		for i := range overhead {
			ovF[i] = float64(overhead[i])
		}
		avg, std := meanStd(ovF)
		fmt.Printf("message_overhead: avg=%.2f std=%.2f\n", avg, std)
	}
}

func parseSeeds(s string, runs int) []int {
	if s == "" {
		return nil
	}
	var out []int
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func startNodes(nodeBin string, n, basePort int, runDir string, seed int) ([]*exec.Cmd, *bufio.Writer) {
	var cmds []*exec.Cmd
	var stdinPipe *bufio.Writer
	bootstrap := fmt.Sprintf("127.0.0.1:%d", basePort)

	for i := 0; i < n; i++ {
		port := basePort + i
		logPath := filepath.Join(runDir, fmt.Sprintf("node_%d.log", i))
		args := []string{
			"-port", strconv.Itoa(port),
			"-fanout", "3", "-ttl", "8", "-peer-limit", "20",
			"-ping-interval", "2s", "-peer-timeout", "6s",
			"-seed", strconv.Itoa(seed),
			"-logfile", logPath,
		}
		if i > 0 {
			args = append(args, "-bootstrap", bootstrap)
		}
		cmd := exec.Command(nodeBin, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		injectorIndex := 1
		if n == 1 {
			injectorIndex = 0
		}
		if i == injectorIndex {
			stdin, err := cmd.StdinPipe()
			if err != nil {
				for _, c := range cmds {
					_ = c.Process.Kill()
				}
				return nil, nil
			}
			stdinPipe = bufio.NewWriter(stdin)
		}
		if err := cmd.Start(); err != nil {
			for _, c := range cmds {
				_ = c.Process.Kill()
			}
			return nil, nil
		}
		cmds = append(cmds, cmd)
	}
	return cmds, stdinPipe
}

// parseRunLogs reads structured logs from runDir, finds the injected gossip by topic=sim,
// returns convergence time in ms from origin until 95% of nodes received, and message overhead (GOSSIP sends).
func parseRunLogs(runDir string, numNodes int) (convergenceMs int64, overhead int) {
	var originMsgID string
	var originTimeMs int64
	var receiveTimes []int64

	for i := 0; i < numNodes; i++ {
		f, err := os.Open(filepath.Join(runDir, fmt.Sprintf("node_%d.log", i)))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var ev logger.Event
			if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
				continue
			}
			if ev.Event == logger.EvGossipOrigin && ev.Payload["topic"] == "sim" && originMsgID == "" {
				originMsgID = ev.Payload["msg_id"]
				originTimeMs = ev.TimeMs
			}
			if ev.Event == logger.EvGossipReceive && ev.Payload["msg_id"] == originMsgID {
				receiveTimes = append(receiveTimes, ev.TimeMs)
			}
			if ev.Event == logger.EvMessageSent && ev.Payload["type"] == "GOSSIP" && ev.Payload["msg_id"] == originMsgID {
				overhead++
			}
		}
		_ = f.Close()
	}

	if originMsgID == "" || len(receiveTimes) == 0 {
		return -1, overhead
	}
	sortInt64(receiveTimes)
	idx := int(math.Ceil(0.95*float64(numNodes))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(receiveTimes) {
		idx = len(receiveTimes) - 1
	}
	return receiveTimes[idx] - originTimeMs, overhead
}

func sortInt64(a []int64) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if a[j] < a[i] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

func meanStd(x []float64) (mean, std float64) {
	if len(x) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range x {
		sum += v
	}
	mean = sum / float64(len(x))
	var sq float64
	for _, v := range x {
		sq += (v - mean) * (v - mean)
	}
	if len(x) > 1 {
		std = math.Sqrt(sq / float64(len(x)-1))
	}
	return mean, std
}
