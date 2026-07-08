package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
)

type Hop struct {
	TTL         uint32
	Host        string // IP; empty = anonymous (no reply)
	LossPercent float64
	AvgRttMs    float64
	BestRttMs   float64
	WorstRttMs  float64
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
	_, ok := os.LookupEnv("NETLAMA_TRACEROUTE_DEMO")
	return ok
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

	// Resolve the target so we can tell "reached the destination" from
	// "reached some other last-responding hop".
	targetIP := target
	if ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", target); err == nil && len(ips) > 0 {
		targetIP = ips[0].String()
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

	res, err := parseMTR(out, target, targetIP)
	if err != nil {
		return nil, err
	}
	return res, nil
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
		} `json:"hubs"`
	} `json:"report"`
}

func parseMTR(data []byte, target, targetIP string) (*TracerouteResult, error) {
	var m mtrReport
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing mtr json: %w", err)
	}

	res := &TracerouteResult{Target: target, TargetIP: targetIP}
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
			Sent:        uint32(h.Snt),
		}
		res.Hops = append(res.Hops, hop)
		if host != "" && h.Loss < 100 {
			hcopy := hop
			lastResponder = &hcopy
		}
	}

	// Reached if the destination IP appears as a responding final hop.
	if len(res.Hops) > 0 {
		last := res.Hops[len(res.Hops)-1]
		if last.Host != "" && last.LossPercent < 100 &&
			(last.Host == targetIP || last.Host == target) {
			res.Reached = true
			res.Status = "reached"
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
