package testtype

import pb "github.com/fox27374/net-lama/proto"

// This file is the one place a test type is described. Adding a type means
// adding an init() entry here plus its proto message and probe — no switch
// statement elsewhere needs to learn about it.

func init() {
	register(&Spec{
		Type:      "ping",
		Unit:      "ms",
		NewParams: func() Params { return &PingParams{} },
		Primary:   func(r *pb.TestResult) (float64, bool) { return positive(r.GetPing().GetAvgRttMs()) },
		Metrics: map[string]Metric{
			"latency_ms":   func(r *pb.TestResult) (float64, bool) { return r.GetPing().GetAvgRttMs(), true },
			"loss_percent": func(r *pb.TestResult) (float64, bool) { return r.GetPing().GetLossPercent(), true },
		},
		Subject: func(r *pb.TestResult) string { return r.GetPing().GetTarget() },
	})

	register(&Spec{
		Type:      "dns",
		Unit:      "ms",
		NewParams: func() Params { return &DNSParams{} },
		Primary:   func(r *pb.TestResult) (float64, bool) { return positive(r.GetDns().GetResolveTimeMs()) },
		Metrics: map[string]Metric{
			"latency_ms": func(r *pb.TestResult) (float64, bool) { return r.GetDns().GetResolveTimeMs(), true },
		},
		Subject: func(r *pb.TestResult) string {
			return r.GetDns().GetQuery() + "@" + r.GetDns().GetServer()
		},
	})

	register(&Spec{
		Type:      "http",
		Unit:      "ms",
		NewParams: func() Params { return &HTTPParams{} },
		Primary:   func(r *pb.TestResult) (float64, bool) { return positive(r.GetHttp().GetTotalMs()) },
		Metrics: map[string]Metric{
			"latency_ms": func(r *pb.TestResult) (float64, bool) { return r.GetHttp().GetTotalMs(), true },
		},
		Subject: func(r *pb.TestResult) string { return r.GetHttp().GetUrl() },
	})

	register(&Spec{
		Type:      "tcp",
		Unit:      "ms",
		NewParams: func() Params { return &TCPParams{} },
		Primary:   func(r *pb.TestResult) (float64, bool) { return positive(r.GetTcp().GetConnectMs()) },
		Metrics: map[string]Metric{
			"latency_ms": func(r *pb.TestResult) (float64, bool) { return r.GetTcp().GetConnectMs(), true },
		},
		Subject: func(r *pb.TestResult) string { return r.GetTcp().GetTarget() },
	})

	// saas reuses the http and tcp result messages rather than duplicating
	// their fields, so every reader here has to handle both. That is also
	// why a result's type comes from its test definition and not from the
	// payload shape — see docs/adr/0001-test-type-from-definition.md.
	register(&Spec{
		Type:               "saas",
		Unit:               "ms",
		MinIntervalSeconds: 60,
		NewParams:          func() Params { return &SaasParams{} },
		Primary:            saasLatency,
		Metrics: map[string]Metric{
			"latency_ms": saasLatency,
			// The whole fetch, redirects and body included — worth alerting
			// on when a front door gets slow to *finish*, not just to start.
			// https endpoints only; a tcp endpoint has nothing to total.
			"total_ms": func(r *pb.TestResult) (float64, bool) {
				if r.GetHttp() == nil {
					return 0, false
				}
				return positive(r.GetHttp().GetTotalMs())
			},
			// https endpoints only; a tcp endpoint reports no certificate.
			"cert_expiry_days": func(r *pb.TestResult) (float64, bool) {
				if r.GetHttp() == nil {
					return 0, false
				}
				return r.GetHttp().GetCertExpiryDays(), true
			},
		},
		Subject: func(r *pb.TestResult) string {
			if url := r.GetHttp().GetUrl(); url != "" {
				return url
			}
			return r.GetTcp().GetTarget()
		},
	})

	register(&Spec{
		Type:               "speedtest",
		Unit:               "Mbps",
		NewParams:          func() Params { return &SpeedtestParams{} },
		MinIntervalSeconds: 60,
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
		Type:      "perfmon",
		Unit:      "Mbps",
		NewParams: func() Params { return &PerfmonParams{} },
		// Each direction takes durationSeconds; keep the test rare
		// enough that it stays a small fraction of the schedule.
		MinIntervalSeconds: 60,
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
		Type:               "traceroute",
		Unit:               "hops",
		NewParams:          func() Params { return &TracerouteParams{} },
		MinIntervalSeconds: 30,
		// Path length is the primary signal: a route that grows has changed.
		Primary: func(r *pb.TestResult) (float64, bool) {
			return positive(float64(len(r.GetTraceroute().GetHops())))
		},
		Metrics: map[string]Metric{
			"latency_ms": func(r *pb.TestResult) (float64, bool) { return r.GetTraceroute().GetRttMs(), true },
			// 1 on a run whose route differs from the previous one. Set by
			// the server on ingest, not by the agent — see
			// internal/server/pathchange.go. A rule of "path_changed > 0"
			// alerts on reroutes with no new alerting machinery.
			"path_changed": func(r *pb.TestResult) (float64, bool) {
				if r.GetTraceroute() == nil {
					return 0, false
				}
				if r.GetTraceroute().GetPathChanged() {
					return 1, true
				}
				return 0, true
			},
		},
		Subject: func(r *pb.TestResult) string { return r.GetTraceroute().GetTarget() },
	})

	register(&Spec{
		Type: "wlan_passive",
		// Passive scanning needs monitor mode, which agents report as the
		// generic "wlan" capability rather than under this type's name.
		Capability:         "wlan",
		Unit:               "%",
		NewParams:          func() Params { return &WlanPassiveParams{} },
		MinIntervalSeconds: 60,
		// Busiest channel in the sweep. The alert engine used to read the
		// number of networks heard instead, so a wlan_passive threshold
		// meant one thing on the dashboard and another to alerting.
		Primary: func(r *pb.TestResult) (float64, bool) { return positive(maxChannelUtil(r)) },
		Metrics: map[string]Metric{
			"utilization_pct": func(r *pb.TestResult) (float64, bool) { return maxChannelUtil(r), true },
		},
	})

	register(&Spec{
		Type:      "wlan_active",
		Unit:      "ms",
		NewParams: func() Params { return &WlanActiveParams{} },
		// The test takes the radio away from passive sweeps; keep it rare.
		MinIntervalSeconds: 300,
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

// saasLatency is how long one saas endpoint took to start answering: time
// to first byte for an https endpoint, the TCP handshake for a tcp one.
//
// Deliberately not the total: a saas endpoint is a vendor's front door, and
// its total is dominated by redirect hops and a few hundred KB of landing
// page (teams.microsoft.com is 3 redirects and ~230 KB), so it moves when
// Microsoft reworks a page rather than when the service degrades. TTFB
// answers "is the service responding to this site", which is what a
// threshold on a reachability check should watch. The total is still
// recorded in every result and available as the total_ms metric.
func saasLatency(r *pb.TestResult) (float64, bool) {
	if r.GetHttp() != nil {
		return positive(r.GetHttp().GetTtfbMs())
	}
	return positive(r.GetTcp().GetConnectMs())
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
