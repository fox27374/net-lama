package testtype

import pb "github.com/fox27374/net-lama/proto"

// This file is the one place a test type is described. Adding a type means
// adding an init() entry here plus its proto message and probe — no switch
// statement elsewhere needs to learn about it.

func init() {
	register(&Spec{
		Type:    "ping",
		Unit:    "ms",
		Primary: func(r *pb.TestResult) (float64, bool) { return positive(r.GetPing().GetAvgRttMs()) },
		Metrics: map[string]Metric{
			"latency_ms":   func(r *pb.TestResult) (float64, bool) { return r.GetPing().GetAvgRttMs(), true },
			"loss_percent": func(r *pb.TestResult) (float64, bool) { return r.GetPing().GetLossPercent(), true },
		},
		Subject: func(r *pb.TestResult) string { return r.GetPing().GetTarget() },
	})

	register(&Spec{
		Type:    "dns",
		Unit:    "ms",
		Primary: func(r *pb.TestResult) (float64, bool) { return positive(r.GetDns().GetResolveTimeMs()) },
		Metrics: map[string]Metric{
			"latency_ms": func(r *pb.TestResult) (float64, bool) { return r.GetDns().GetResolveTimeMs(), true },
		},
		Subject: func(r *pb.TestResult) string {
			return r.GetDns().GetQuery() + "@" + r.GetDns().GetServer()
		},
	})

	register(&Spec{
		Type:    "http",
		Unit:    "ms",
		Primary: func(r *pb.TestResult) (float64, bool) { return positive(r.GetHttp().GetTotalMs()) },
		Metrics: map[string]Metric{
			"latency_ms": func(r *pb.TestResult) (float64, bool) { return r.GetHttp().GetTotalMs(), true },
		},
		Subject: func(r *pb.TestResult) string { return r.GetHttp().GetUrl() },
	})

	register(&Spec{
		Type:    "tcp",
		Unit:    "ms",
		Primary: func(r *pb.TestResult) (float64, bool) { return positive(r.GetTcp().GetConnectMs()) },
		Metrics: map[string]Metric{
			"latency_ms": func(r *pb.TestResult) (float64, bool) { return r.GetTcp().GetConnectMs(), true },
		},
		Subject: func(r *pb.TestResult) string { return r.GetTcp().GetTarget() },
	})

	register(&Spec{
		Type: "speedtest",
		Unit: "Mbps",
		// Throughput degrades by falling, so warn > crit for this type.
		LowerIsWorse: true,
		Primary:      func(r *pb.TestResult) (float64, bool) { return positive(r.GetSpeedtest().GetDownloadMbps()) },
		Metrics: map[string]Metric{
			"latency_ms":    func(r *pb.TestResult) (float64, bool) { return r.GetSpeedtest().GetLatencyMs(), true },
			"download_mbps": func(r *pb.TestResult) (float64, bool) { return r.GetSpeedtest().GetDownloadMbps(), true },
			"upload_mbps":   func(r *pb.TestResult) (float64, bool) { return r.GetSpeedtest().GetUploadMbps(), true },
		},
	})

	register(&Spec{
		Type: "perfmon",
		Unit: "Mbps",
		// NOTE: perfmon measures throughput like speedtest and arguably
		// wants LowerIsWorse too, but it has always been evaluated
		// higher-is-worse. Flipping it here would silently invert every
		// existing perfmon threshold, so it stays as-is until someone
		// decides to migrate them.
		Primary: func(r *pb.TestResult) (float64, bool) { return positive(r.GetPerfmon().GetDownloadMbps()) },
		Metrics: map[string]Metric{
			"latency_ms":    func(r *pb.TestResult) (float64, bool) { return r.GetPerfmon().GetLatencyMs(), true },
			"download_mbps": func(r *pb.TestResult) (float64, bool) { return r.GetPerfmon().GetDownloadMbps(), true },
			"upload_mbps":   func(r *pb.TestResult) (float64, bool) { return r.GetPerfmon().GetUploadMbps(), true },
		},
		Subject: func(r *pb.TestResult) string { return r.GetPerfmon().GetTarget() },
	})

	register(&Spec{
		Type: "traceroute",
		Unit: "hops",
		// Path length is the primary signal: a route that grows has changed.
		Primary: func(r *pb.TestResult) (float64, bool) {
			return positive(float64(len(r.GetTraceroute().GetHops())))
		},
		Metrics: map[string]Metric{
			"latency_ms": func(r *pb.TestResult) (float64, bool) { return r.GetTraceroute().GetRttMs(), true },
		},
		Subject: func(r *pb.TestResult) string { return r.GetTraceroute().GetTarget() },
	})

	register(&Spec{
		Type: "wlan_passive",
		// Passive scanning needs monitor mode, which agents report as the
		// generic "wlan" capability rather than under this type's name.
		Capability: "wlan",
		Unit:       "%",
		// Busiest channel in the sweep. The alert engine used to read the
		// number of networks heard instead, so a wlan_passive threshold
		// meant one thing on the dashboard and another to alerting.
		Primary: func(r *pb.TestResult) (float64, bool) { return positive(maxChannelUtil(r)) },
		Metrics: map[string]Metric{
			"utilization_pct": func(r *pb.TestResult) (float64, bool) { return maxChannelUtil(r), true },
		},
	})

	register(&Spec{
		Type: "wlan_active",
		Unit: "ms",
		// Time to a usable connection: associate + authenticate + DHCP.
		// Scan time is excluded — SSID discovery is harness-internal and
		// its variance would drown the signal.
		Primary: func(r *pb.TestResult) (float64, bool) { return positive(connectMs(r)) },
		Metrics: map[string]Metric{
			"latency_ms":   func(r *pb.TestResult) (float64, bool) { return connectMs(r), true },
			"loss_percent": func(r *pb.TestResult) (float64, bool) { return r.GetWlanActive().GetGatewayPingLossPct(), true },
		},
		Subject: func(r *pb.TestResult) string { return r.GetWlanActive().GetSsid() },
	})
}

// resultTypeName maps a result's payload oneof to its type name. This is
// the one switch the registry cannot express as data — add a case here
// whenever a TestResult_* variant is added, or results of the new type
// store as "unknown" and drop out of the ?type= filter the UI queries by.
func resultTypeName(r *pb.TestResult) string {
	switch r.Result.(type) {
	case *pb.TestResult_Ping:
		return "ping"
	case *pb.TestResult_Dns:
		return "dns"
	case *pb.TestResult_Http:
		return "http"
	case *pb.TestResult_Tcp:
		return "tcp"
	case *pb.TestResult_Speedtest:
		return "speedtest"
	case *pb.TestResult_Perfmon:
		return "perfmon"
	case *pb.TestResult_Traceroute:
		return "traceroute"
	case *pb.TestResult_WlanPassive:
		return "wlan_passive"
	case *pb.TestResult_WlanActive:
		return "wlan_active"
	}
	return ""
}

// maxChannelUtil is the busiest channel's utilization in a passive sweep.
func maxChannelUtil(r *pb.TestResult) float64 {
	var max float64
	for _, ch := range r.GetWlanPassive().GetChannels() {
		if ch.GetUtilizationPct() > max {
			max = ch.GetUtilizationPct()
		}
	}
	return max
}

// connectMs is a wlan_active run's time to a usable connection.
func connectMs(r *pb.TestResult) float64 {
	wa := r.GetWlanActive()
	return wa.GetAssociateMs() + wa.GetAuthenticateMs() + wa.GetDhcpMs()
}
