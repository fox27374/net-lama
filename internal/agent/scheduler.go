package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fox27374/net-lama/internal/probe"
	pb "github.com/fox27374/net-lama/proto"
)

// schedule reacts to config updates and commands. Every config update
// replaces the running test schedule completely.
func (a *Agent) schedule(ctx context.Context, cfgCh <-chan *pb.Config, cmdCh <-chan *pb.Command, results chan<- *pb.TestResult) {
	specs := map[string]*pb.TestSpec{}
	var cancelTests context.CancelFunc

	defer func() {
		if cancelTests != nil {
			cancelTests()
		}
	}()

	for {
		select {
		case cfg := <-cfgCh:
			if cancelTests != nil {
				cancelTests()
			}
			testCtx, cancel := context.WithCancel(ctx)
			cancelTests = cancel

			specs = map[string]*pb.TestSpec{}
			for _, spec := range cfg.Tests {
				specs[spec.Id] = spec
				a.startTest(testCtx, spec, results)
			}
			if len(cfg.Tests) == 0 {
				a.Logger.Info("No tests assigned, idling")
			}

		case cmd := <-cmdCh:
			if cmd.Type == pb.Command_RUN_TEST {
				if spec, ok := specs[cmd.TestId]; ok {
					go a.runTest(ctx, spec, results)
				} else {
					a.Logger.Warn("Command for unknown test", slog.String("testId", cmd.TestId))
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) startTest(ctx context.Context, spec *pb.TestSpec, results chan<- *pb.TestResult) {
	a.Logger.Info("Scheduling test",
		slog.String("test", spec.Name),
		slog.Uint64("intervalSeconds", uint64(spec.IntervalSeconds)),
	)
	go runEvery(ctx, spec.IntervalSeconds, func() { a.runTest(ctx, spec, results) })
}

// runEvery runs fn immediately and then on every tick until ctx is cancelled.
func runEvery(ctx context.Context, intervalSeconds uint32, fn func()) {
	if intervalSeconds == 0 {
		intervalSeconds = 60
	}
	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()

	fn()
	for {
		select {
		case <-ticker.C:
			fn()
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) runTest(ctx context.Context, spec *pb.TestSpec, results chan<- *pb.TestResult) {
	switch params := spec.Params.(type) {
	case *pb.TestSpec_Speedtest:
		a.runSpeedtest(ctx, spec, results)
	case *pb.TestSpec_Ping:
		a.runPings(ctx, spec, params.Ping, results)
	case *pb.TestSpec_Dns:
		a.runDNS(ctx, spec, params.Dns, results)
	case *pb.TestSpec_Http:
		a.runHTTP(ctx, spec, params.Http, results)
	case *pb.TestSpec_Tcp:
		a.runTCP(ctx, spec, params.Tcp, results)
	case *pb.TestSpec_WlanPassive:
		a.runWlanPassive(ctx, spec, results)
	case *pb.TestSpec_Traceroute:
		a.runTraceroute(ctx, spec, params.Traceroute, results)
	}
}

func newResult(spec *pb.TestSpec) *pb.TestResult {
	return &pb.TestResult{
		Time:     timestamppb.Now(),
		TestId:   spec.Id,
		TestName: spec.Name,
	}
}

// speedtestProviders maps the provider param to the probe function that
// implements it. An empty provider means "ookla" (backward compatible with
// every existing test row, which predates the provider field).
var speedtestProviders = map[string]func(context.Context) (*probe.SpeedtestResult, error){
	"":           probe.Speedtest,
	"ookla":      probe.Speedtest,
	"ndt7":       probe.NDT7,
	"cloudflare": probe.Cloudflare,
}

func (a *Agent) runSpeedtest(ctx context.Context, spec *pb.TestSpec, results chan<- *pb.TestResult) {
	params := spec.GetSpeedtest()
	provider := params.GetProvider()
	runner, ok := speedtestProviders[provider]
	if !ok {
		// Config validation on the server should prevent this, but fall
		// back to ookla rather than dropping the test entirely.
		runner = probe.Speedtest
	}
	reportedProvider := provider
	if reportedProvider == "" {
		reportedProvider = "ookla"
	}

	a.Logger.Info("Running speedtest", slog.String("test", spec.Name), slog.String("provider", reportedProvider))
	res, err := runner(ctx)

	result := newResult(spec)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		a.Logger.Error("Speedtest failed",
			slog.String("test", spec.Name), slog.String("provider", reportedProvider), slog.Any("error", err))
		result.Error = err.Error()
		result.Result = &pb.TestResult_Speedtest{Speedtest: &pb.SpeedtestResult{Provider: reportedProvider}}
	} else {
		a.Logger.Info("Speedtest done",
			slog.String("test", spec.Name),
			slog.String("provider", reportedProvider),
			slog.Float64("downloadMbps", res.DownloadMbps),
			slog.Float64("uploadMbps", res.UploadMbps),
			slog.Float64("latencyMs", res.LatencyMs),
		)
		result.Result = &pb.TestResult_Speedtest{Speedtest: &pb.SpeedtestResult{
			ServerName:    res.ServerName,
			ServerCountry: res.ServerCountry,
			LatencyMs:     res.LatencyMs,
			DownloadMbps:  res.DownloadMbps,
			UploadMbps:    res.UploadMbps,
			UserIp:        res.UserIP,
			UserIsp:       res.UserISP,
			Provider:      reportedProvider,
		}}
	}
	sendResult(ctx, results, result)
}

func (a *Agent) runPings(ctx context.Context, spec *pb.TestSpec, params *pb.PingParams, results chan<- *pb.TestResult) {
	for _, target := range params.Targets {
		res, err := probe.Ping(ctx, target, int(params.Count))

		result := newResult(spec)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			a.Logger.Error("Ping failed",
				slog.String("test", spec.Name),
				slog.String("target", target),
				slog.Any("error", err),
			)
			result.Error = err.Error()
			result.Result = &pb.TestResult_Ping{Ping: &pb.PingResult{Target: target}}
		} else {
			result.Result = &pb.TestResult_Ping{Ping: &pb.PingResult{
				Target:          target,
				PacketsSent:     uint32(res.PacketsSent),
				PacketsReceived: uint32(res.PacketsReceived),
				LossPercent:     res.LossPercent,
				MinRttMs:        res.MinRttMs,
				AvgRttMs:        res.AvgRttMs,
				MaxRttMs:        res.MaxRttMs,
			}}
		}
		sendResult(ctx, results, result)
	}
}

func (a *Agent) runDNS(ctx context.Context, spec *pb.TestSpec, params *pb.DnsParams, results chan<- *pb.TestResult) {
	for _, server := range params.Servers {
		for _, query := range params.Queries {
			res := probe.DNSQuery(ctx, query, server)
			if ctx.Err() != nil {
				return
			}

			result := newResult(spec)
			result.Result = &pb.TestResult_Dns{Dns: &pb.DnsResult{
				Query:         res.Query,
				Server:        res.Server,
				Success:       res.Success,
				ResolveTimeMs: res.ResolveTimeMs,
				Addresses:     res.Addresses,
			}}
			sendResult(ctx, results, result)
		}
	}
}

func (a *Agent) runHTTP(ctx context.Context, spec *pb.TestSpec, params *pb.HttpParams, results chan<- *pb.TestResult) {
	res, err := probe.HTTPCheck(ctx, params.Url, params.TimeoutSeconds, params.SkipTlsVerify)

	result := newResult(spec)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		a.Logger.Error("HTTP check failed",
			slog.String("test", spec.Name),
			slog.String("url", params.Url),
			slog.Any("error", err),
		)
		result.Error = err.Error()
		result.Result = &pb.TestResult_Http{Http: &pb.HttpResult{Url: params.Url, CertExpiryDays: -1}}
	} else {
		result.Result = &pb.TestResult_Http{Http: &pb.HttpResult{
			Url:            res.URL,
			StatusCode:     uint32(res.StatusCode),
			DnsMs:          res.DNSMs,
			ConnectMs:      res.ConnectMs,
			TlsMs:          res.TLSMs,
			TtfbMs:         res.TTFBMs,
			TotalMs:        res.TotalMs,
			CertExpiryDays: res.CertExpiryDays,
			ServerIp:       res.ServerIP,
		}}
	}
	sendResult(ctx, results, result)
}

func (a *Agent) runTCP(ctx context.Context, spec *pb.TestSpec, params *pb.TcpParams, results chan<- *pb.TestResult) {
	for _, target := range params.Targets {
		res, err := probe.TCPConnect(ctx, target, params.TimeoutSeconds)
		if ctx.Err() != nil {
			return
		}

		result := newResult(spec)
		result.Result = &pb.TestResult_Tcp{Tcp: &pb.TcpResult{
			Target:    res.Target,
			Connected: res.Connected,
			ConnectMs: res.ConnectMs,
		}}
		if err != nil {
			result.Error = err.Error()
		}
		sendResult(ctx, results, result)
	}
}

func (a *Agent) runWlanPassive(ctx context.Context, spec *pb.TestSpec, results chan<- *pb.TestResult) {
	result := newResult(spec)

	// Use override if set; otherwise auto-pick the first monitor-capable interface.
	iface := a.WlanIface
	if iface == "" {
		if detected := probe.WirelessInterfaces(ctx); len(detected) > 0 {
			for _, d := range detected {
				if d.SupportsMonitor {
					iface = d.Name
					break
				}
			}
		}
	} else {
		// Validate override: check that the interface exists and is monitor-capable.
		if detected := probe.WirelessInterfaces(ctx); len(detected) > 0 {
			found := false
			for _, d := range detected {
				if d.Name == iface {
					found = true
					if !d.SupportsMonitor {
						a.Logger.Warn("WLAN passive skipped: interface is not monitor-capable", slog.String("test", spec.Name), slog.String("interface", iface))
						result.Error = fmt.Sprintf("interface %q is not monitor-capable", iface)
						result.Result = &pb.TestResult_WlanPassive{WlanPassive: &pb.WlanPassiveResult{}}
						sendResult(ctx, results, result)
						return
					}
					break
				}
			}
			if !found {
				a.Logger.Warn("WLAN passive skipped: interface not found", slog.String("test", spec.Name), slog.String("interface", iface))
				result.Error = fmt.Sprintf("interface %q not found", iface)
				result.Result = &pb.TestResult_WlanPassive{WlanPassive: &pb.WlanPassiveResult{}}
				sendResult(ctx, results, result)
				return
			}
		}
	}

	// Demo mode synthesizes data and needs no real interface.
	if iface == "" && !probe.DemoMode() {
		a.Logger.Warn("WLAN passive skipped: no monitor-capable interface", slog.String("test", spec.Name))
		result.Error = "no monitor-capable wireless interface available"
		result.Result = &pb.TestResult_WlanPassive{WlanPassive: &pb.WlanPassiveResult{}}
		sendResult(ctx, results, result)
		return
	}

	// Determine whether to do a full sweep or narrow sweep based on agent state
	a.wlanMu.Lock()
	testKey := spec.Id
	state, hasState := a.wlanState[testKey]

	var channels []uint32
	if !hasState || len(state.InterestingChannels) == 0 {
		// First run or no interesting channels yet: full sweep
		channels = nil // nil means all channels
		a.Logger.Info("WLAN passive sweep starting (full spectrum)", slog.String("test", spec.Name), slog.String("interface", iface))
	} else {
		// Subsequent runs: narrow to interesting channels
		channels = state.InterestingChannels
		a.Logger.Info("WLAN passive sweep starting (narrowed channels)",
			slog.String("test", spec.Name), slog.String("interface", iface),
			slog.Int("channelCount", len(channels)))
	}

	usedIface, stations, channelStats, networks, sweepMs, err := probe.Sense(ctx, iface, channels, 400)
	if err != nil {
		a.wlanMu.Unlock()
		if ctx.Err() != nil {
			return
		}
		a.Logger.Error("WLAN passive failed",
			slog.String("test", spec.Name), slog.String("interface", iface), slog.Any("error", err))
		result.Error = err.Error()
		result.Result = &pb.TestResult_WlanPassive{WlanPassive: &pb.WlanPassiveResult{Interface: iface}}
		sendResult(ctx, results, result)
		return
	}

	// Update interesting channels from this sweep
	interestingChannels := extractInterestingChannels(networks, stations)
	if len(interestingChannels) == 0 && hasState && len(state.InterestingChannels) > 0 {
		// If this sweep found nothing but we had previous results, keep the old set
		interestingChannels = state.InterestingChannels
	}

	if a.wlanState == nil {
		a.wlanState = make(map[string]*wlanPassiveState)
	}
	a.wlanState[testKey] = &wlanPassiveState{
		InterestingChannels: interestingChannels,
	}
	a.wlanMu.Unlock()

	a.Logger.Info("WLAN passive done",
		slog.String("test", spec.Name), slog.String("interface", usedIface),
		slog.Int("stations", len(stations)), slog.Int("networks", len(networks)),
		slog.Uint64("sweepMs", uint64(sweepMs)))
	result.Result = wlanPassiveResult(usedIface, stations, channelStats, networks, sweepMs)
	sendResult(ctx, results, result)
}

// extractInterestingChannels returns the set of channels where APs or stations were heard
func extractInterestingChannels(networks []probe.WlanNetwork, stations []probe.WlanStation) []uint32 {
	seen := make(map[uint32]bool)
	for _, n := range networks {
		if n.Channel > 0 {
			seen[n.Channel] = true
		}
	}
	var result []uint32
	for ch := range seen {
		result = append(result, ch)
	}
	// Sort for deterministic ordering
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

// wlanPassiveResult converts probe results into the protobuf WlanPassiveResult payload.
func wlanPassiveResult(iface string, stations []probe.WlanStation, channelStats []probe.WlanChannelStat, networks []probe.WlanNetwork, sweepMs uint32) *pb.TestResult_WlanPassive {
	pbStations := make([]*pb.WlanStation, 0, len(stations))
	for _, st := range stations {
		pbStations = append(pbStations, &pb.WlanStation{
			Mac:        st.MAC,
			Bssid:      st.BSSID,
			Ssid:       st.SSID,
			RssiDbm:    st.RSSIdBm,
			RssiAvgDbm: st.RSSIAvgdBm,
			RateMbps:   st.RateMbps,
			Mcs:        st.MCS,
			Frames:     st.Frames,
			ProbeOnly:  st.ProbeOnly,
			LastSeenMs: st.LastSeenMs,
		})
	}

	pbChannels := make([]*pb.WlanChannelStat, 0, len(channelStats))
	for _, ch := range channelStats {
		pbChannels = append(pbChannels, &pb.WlanChannelStat{
			Channel:        ch.Channel,
			FreqMhz:        ch.FreqMHz,
			ActiveMs:       ch.ActiveMs,
			BusyMs:         ch.BusyMs,
			UtilizationPct: ch.UtilizationPct,
			Frames:         ch.Frames,
		})
	}

	pbNetworks := make([]*pb.WlanNetwork, 0, len(networks))
	for _, n := range networks {
		pbNetworks = append(pbNetworks, &pb.WlanNetwork{
			Bssid:              n.BSSID,
			Ssid:               n.SSID,
			Channel:            n.Channel,
			FreqMhz:            n.FreqMHz,
			RssiDbm:            n.RSSIdBm,
			Beacons:            n.Beacons,
			Security:           n.Security,
			Standards:          n.Standards,
			WidthMhz:           n.WidthMHz,
			BeaconIntervalTu:   n.BeaconIntervalTU,
			Country:            n.Country,
			LoadPresent:        n.LoadPresent,
			LoadStations:       n.LoadStations,
			LoadChannelUtilPct: n.LoadChannelUtilPct,
			SecurityDetail:     n.SecurityDetail,
			Roaming:            n.Roaming,
			Mfp:                n.MFP,
			GroupCipher:        n.GroupCipher,
			DtimPeriod:         n.DTIMPeriod,
			Wps:                n.WPS,
			Streams:            n.Streams,
			MaxRateMbps:        n.MaxRateMbps,
		})
	}

	return &pb.TestResult_WlanPassive{WlanPassive: &pb.WlanPassiveResult{
		Interface: iface,
		Stations:  pbStations,
		Channels:  pbChannels,
		Networks:  pbNetworks,
		SweepMs:   sweepMs,
		Demo:      probe.DemoMode(),
	}}
}

func (a *Agent) runTraceroute(ctx context.Context, spec *pb.TestSpec, params *pb.TracerouteParams, results chan<- *pb.TestResult) {
	res, err := probe.Traceroute(ctx, params.Target, params.Protocol, params.Port, params.MaxHops, params.ProbesPerHop)

	result := newResult(spec)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		a.Logger.Error("Traceroute failed",
			slog.String("test", spec.Name), slog.String("target", params.Target), slog.Any("error", err))
		result.Error = err.Error()
		result.Result = &pb.TestResult_Traceroute{Traceroute: &pb.TracerouteResult{
			Target: params.Target, Status: "error",
		}}
		sendResult(ctx, results, result)
		return
	}

	hops := make([]*pb.Hop, 0, len(res.Hops))
	for _, h := range res.Hops {
		hops = append(hops, &pb.Hop{
			Ttl:         h.TTL,
			Host:        h.Host,
			HostName:    h.HostName,
			LossPercent: h.LossPercent,
			AvgRttMs:    h.AvgRttMs,
			BestRttMs:   h.BestRttMs,
			WorstRttMs:  h.WorstRttMs,
			JitterMs:    h.JitterMs,
			Sent:        h.Sent,
		})
	}
	a.Logger.Info("Traceroute done",
		slog.String("test", spec.Name), slog.String("target", res.Target),
		slog.Bool("reached", res.Reached), slog.Int("hops", len(hops)))
	result.Result = &pb.TestResult_Traceroute{Traceroute: &pb.TracerouteResult{
		Target:     res.Target,
		TargetIp:   res.TargetIP,
		Reached:    res.Reached,
		Status:     res.Status,
		FailureHop: res.FailureHop,
		RttMs:      res.RttMs,
		Demo:       res.Demo,
		Hops:       hops,
	}}
	sendResult(ctx, results, result)
}

func sendResult(ctx context.Context, results chan<- *pb.TestResult, result *pb.TestResult) {
	select {
	case results <- result:
	case <-ctx.Done():
	}
}
