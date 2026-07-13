package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Hop struct {
	TTL         uint32
	Host        string // IP; empty = anonymous (no reply)
	HostName    string // reverse-DNS name; empty when not resolved or failed
	LossPercent float64
	AvgRttMs    float64
	BestRttMs   float64
	WorstRttMs  float64
	JitterMs    float64
	Sent        uint32
}

type TracerouteResult struct {
	Target     string
	TargetIP   string
	Reached    bool
	Status     string // "reached" | "stalled" | "error"
	FailureHop uint32
	RttMs      float64
	Demo       bool
	Hops       []Hop
}

func tracerouteDemo() bool {
	return envEnabled("NETLAMA_TRACEROUTE_DEMO")
}

// TracerouteDemoMode reports whether traceroute results are synthetic.
func TracerouteDemoMode() bool { return tracerouteDemo() }

// Traceroute traces the path to target using mtr and classifies where a
// failing path breaks. protocol is icmp|tcp|udp.
func Traceroute(ctx context.Context, target, protocol string, port, maxHops, probes uint32) (*TracerouteResult, error) {
	if tracerouteDemo() {
		return demoTraceroute(target, protocol, port), nil
	}
	if maxHops == 0 {
		maxHops = 30
	}
	if probes == 0 {
		probes = 5
	}

	args := []string{"--json", "-n", "-c", strconv.Itoa(int(probes)), "-m", strconv.Itoa(int(maxHops))}
	switch protocol {
	case "tcp":
		args = append(args, "--tcp")
		if port > 0 {
			args = append(args, "--port", strconv.Itoa(int(port)))
		}
	case "udp":
		args = append(args, "--udp")
		if port > 0 {
			args = append(args, "--port", strconv.Itoa(int(port)))
		}
	}
	args = append(args, target)

	out, err := exec.CommandContext(ctx, "mtr", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("running mtr: %w", err)
	}

	res, err := parseMTR(out, target, maxHops)
	if err != nil {
		return nil, err
	}
	resolveHopNames(ctx, res.Hops)
	return res, nil
}

// resolveHopNames performs best-effort parallel reverse-DNS resolution on hop IPs.
// Each lookup has a 1500ms timeout and never fails the test. Names are stored in-place
// with trailing dots stripped; failures or timeouts leave HostName empty.
func resolveHopNames(ctx context.Context, hops []Hop) {
	var wg sync.WaitGroup
	for i := range hops {
		if hops[i].Host == "" {
			continue // anonymous hop; nothing to resolve
		}
		// Check if Host is already a name (not an IP)
		if net.ParseIP(hops[i].Host) == nil {
			continue // already a name, skip resolution
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			lookupCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel()
			names, err := net.DefaultResolver.LookupAddr(lookupCtx, hops[i].Host)
			if err == nil && len(names) > 0 {
				// Take the first name and strip trailing dot
				hops[i].HostName = strings.TrimSuffix(names[0], ".")
			}
		}(i)
	}
	wg.Wait()
}

// mtrReport mirrors the relevant parts of `mtr --json` output.
type mtrReport struct {
	Report struct {
		Mtr struct {
			Dst string `json:"dst"`
		} `json:"mtr"`
		Hubs []struct {
			Count int     `json:"count"`
			Host  string  `json:"host"`
			Loss  float64 `json:"Loss%"`
			Snt   int     `json:"Snt"`
			Avg   float64 `json:"Avg"`
			Best  float64 `json:"Best"`
			Wrst  float64 `json:"Wrst"`
			StDev float64 `json:"StDev"`
		} `json:"hubs"`
	} `json:"report"`
}

func parseMTR(data []byte, target string, maxHops uint32) (*TracerouteResult, error) {
	var m mtrReport
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing mtr json: %w", err)
	}

	res := &TracerouteResult{Target: target}
	var lastResponder *Hop
	for _, h := range m.Report.Hubs {
		host := h.Host
		if host == "???" {
			host = ""
		}
		hop := Hop{
			TTL:         uint32(h.Count),
			Host:        host,
			LossPercent: h.Loss,
			AvgRttMs:    h.Avg,
			BestRttMs:   h.Best,
			WorstRttMs:  h.Wrst,
			JitterMs:    h.StDev,
			Sent:        uint32(h.Snt),
		}
		res.Hops = append(res.Hops, hop)
		if host != "" && h.Loss < 100 {
			hcopy := hop
			lastResponder = &hcopy
		}
	}

	// mtr stops incrementing TTL only once it reaches the destination, so a
	// report that ends before maxHops with a responding final hop means the
	// target was reached — and that final hop is its real address. If it ran
	// all the way to maxHops, the path stalled before the destination.
	// (Intermediate hops may still be anonymous, e.g. routers that don't send
	// ICMP Time Exceeded to TCP-SYN probes — that is not a failure.)
	if len(res.Hops) > 0 {
		last := res.Hops[len(res.Hops)-1]
		if last.Host != "" && last.LossPercent < 100 && uint32(len(res.Hops)) < maxHops {
			res.Reached = true
			res.Status = "reached"
			res.TargetIP = last.Host
			res.RttMs = last.AvgRttMs
		}
	}
	if !res.Reached {
		res.Status = "stalled"
		if lastResponder != nil {
			res.FailureHop = lastResponder.TTL
			res.RttMs = lastResponder.AvgRttMs
		}
	}
	return res, nil
}
