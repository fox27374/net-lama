package probe

import "math/rand"

// Synthetic traceroute data for building/evaluating the path view on
// hosts without raw-socket access, enabled with NETLAMA_TRACEROUTE_DEMO.

func demoTraceroute(target, protocol string, port uint32) *TracerouteResult {
	targetIP := "142.250.185.68"

	// A plausible path: LAN gateway -> ISP -> transit -> destination.
	path := []struct {
		host     string
		hostName string
		base     float64
	}{
		{"192.168.1.1", "gw.demo.lan", 0.6},
		{"10.64.0.1", "", 3.2},
		{"", "", 0},                        // one anonymous hop
		{"84.116.130.1", "core1.demo-isp.net", 8.5}, // ISP edge
		{"213.46.160.42", "", 11.0},
		{"72.14.204.68", "", 14.5}, // transit
		{"108.170.240.1", "", 18.0},
		{targetIP, "", 21.0},
	}

	// Occasionally stall the path partway to exercise the failure view.
	stallAt := -1
	if rand.Intn(4) == 0 {
		stallAt = 4 + rand.Intn(2) // stall after hop 4 or 5
	}

	res := &TracerouteResult{
		Target: target, TargetIP: targetIP, Demo: true,
	}
	var lastResponder *Hop
	for i, p := range path {
		ttl := uint32(i + 1)
		if stallAt >= 0 && i > stallAt {
			res.Hops = append(res.Hops, Hop{TTL: ttl, Host: "", LossPercent: 100, Sent: 5})
			continue
		}
		loss := 0.0
		if p.host == "" {
			loss = 100
		}
		jitter := rand.Float64() * 2
		jitterMs := 0.2 + rand.Float64()*(3.0-0.2) // 0.2-3 ms
		hop := Hop{
			TTL: ttl, Host: p.host, HostName: p.hostName, LossPercent: loss, Sent: 5,
			AvgRttMs:   p.base + jitter,
			BestRttMs:  p.base,
			WorstRttMs: p.base + jitter + 2,
			JitterMs:   jitterMs,
		}
		res.Hops = append(res.Hops, hop)
		if p.host != "" {
			hc := hop
			lastResponder = &hc
		}
	}

	if stallAt < 0 {
		res.Reached = true
		res.Status = "reached"
		res.RttMs = res.Hops[len(res.Hops)-1].AvgRttMs
	} else {
		res.Status = "stalled"
		if lastResponder != nil {
			res.FailureHop = lastResponder.TTL
			res.RttMs = lastResponder.AvgRttMs
		}
	}
	return res
}
