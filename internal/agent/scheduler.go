package agent

import (
	"context"
	"log/slog"
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
	wlanIface := "" // per-agent sensor interface, from the pushed config

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
			wlanIface = cfg.WlanInterface

			specs = map[string]*pb.TestSpec{}
			for _, spec := range cfg.Tests {
				specs[spec.Id] = spec
				a.startTest(testCtx, spec, wlanIface, results)
			}
			if len(cfg.Tests) == 0 {
				a.Logger.Info("No tests assigned, idling")
			}

		case cmd := <-cmdCh:
			if cmd.Type == pb.Command_RUN_TEST {
				if spec, ok := specs[cmd.TestId]; ok {
					go a.runTest(ctx, spec, wlanIface, results)
				} else {
					a.Logger.Warn("Command for unknown test", slog.String("testId", cmd.TestId))
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) startTest(ctx context.Context, spec *pb.TestSpec, wlanIface string, results chan<- *pb.TestResult) {
	a.Logger.Info("Scheduling test",
		slog.String("test", spec.Name),
		slog.Uint64("intervalSeconds", uint64(spec.IntervalSeconds)),
	)
	go runEvery(ctx, spec.IntervalSeconds, func() { a.runTest(ctx, spec, wlanIface, results) })
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

func (a *Agent) runTest(ctx context.Context, spec *pb.TestSpec, wlanIface string, results chan<- *pb.TestResult) {
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
	case *pb.TestSpec_WlanScan:
		a.runWlanScan(ctx, spec, wlanIface, results)
	case *pb.TestSpec_WlanSense:
		a.runWlanSense(ctx, spec, params.WlanSense, wlanIface, results)
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

func (a *Agent) runWlanScan(ctx context.Context, spec *pb.TestSpec, wlanIface string, results chan<- *pb.TestResult) {
	iface := wlanIface
	if iface == "" {
		// No interface selected: fall back to the first detected one.
		if detected := probe.WirelessInterfaces(ctx); len(detected) > 0 {
			iface = detected[0].Name
		}
	}

	result := newResult(spec)
	if iface == "" {
		a.Logger.Warn("WLAN scan skipped: no wireless interface", slog.String("test", spec.Name))
		result.Error = "no wireless interface available"
		result.Result = &pb.TestResult_WlanScan{WlanScan: &pb.WlanScanResult{}}
		sendResult(ctx, results, result)
		return
	}

	usedIface, aps, err := probe.Scan(ctx, iface)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		a.Logger.Error("WLAN scan failed",
			slog.String("test", spec.Name), slog.String("interface", iface), slog.Any("error", err))
		result.Error = err.Error()
		result.Result = &pb.TestResult_WlanScan{WlanScan: &pb.WlanScanResult{Interface: iface}}
		sendResult(ctx, results, result)
		return
	}

	pbAPs := make([]*pb.AccessPoint, 0, len(aps))
	for _, ap := range aps {
		pbAPs = append(pbAPs, &pb.AccessPoint{
			Bssid:    ap.BSSID,
			Ssid:     ap.SSID,
			Channel:  ap.Channel,
			FreqMhz:  ap.FreqMHz,
			Band:     ap.Band,
			RssiDbm:  ap.RSSIdBm,
			Security: ap.Security,
		})
	}
	a.Logger.Info("WLAN scan done",
		slog.String("test", spec.Name), slog.String("interface", usedIface), slog.Int("aps", len(pbAPs)))
	result.Result = &pb.TestResult_WlanScan{WlanScan: &pb.WlanScanResult{
		Interface: usedIface, AccessPoints: pbAPs, Demo: probe.DemoMode(),
	}}
	sendResult(ctx, results, result)
}

func (a *Agent) runWlanSense(ctx context.Context, spec *pb.TestSpec, params *pb.WlanSenseParams, wlanIface string, results chan<- *pb.TestResult) {
	iface := wlanIface
	if iface == "" {
		// No interface selected: fall back to the first detected monitor-capable one.
		if detected := probe.WirelessInterfaces(ctx); len(detected) > 0 {
			for _, d := range detected {
				if d.SupportsMonitor {
					iface = d.Name
					break
				}
			}
		}
	}

	result := newResult(spec)
	// Demo mode synthesizes data and needs no real interface.
	if iface == "" && !probe.DemoModeWlanSense() {
		a.Logger.Warn("WLAN sense skipped: no monitor-capable interface", slog.String("test", spec.Name))
		result.Error = "no monitor-capable wireless interface available"
		result.Result = &pb.TestResult_WlanSense{WlanSense: &pb.WlanSenseResult{}}
		sendResult(ctx, results, result)
		return
	}

	channels := params.Channels
	dwell := params.DwellMs
	if dwell == 0 {
		dwell = 400
	}

	usedIface, stations, channelStats, networks, sweepMs, err := probe.Sense(ctx, iface, channels, dwell)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		a.Logger.Error("WLAN sense failed",
			slog.String("test", spec.Name), slog.String("interface", iface), slog.Any("error", err))
		result.Error = err.Error()
		result.Result = &pb.TestResult_WlanSense{WlanSense: &pb.WlanSenseResult{Interface: iface}}
		sendResult(ctx, results, result)
		return
	}

	pbStations := make([]*pb.WlanStation, 0, len(stations))
	for _, st := range stations {
		pbStations = append(pbStations, &pb.WlanStation{
			Mac:         st.MAC,
			Bssid:       st.BSSID,
			Ssid:        st.SSID,
			RssiDbm:     st.RSSIdBm,
			RssiAvgDbm:  st.RSSIAvgdBm,
			RateMbps:    st.RateMbps,
			Mcs:         st.MCS,
			Frames:      st.Frames,
			ProbeOnly:   st.ProbeOnly,
			LastSeenMs:  st.LastSeenMs,
		})
	}

	pbChannels := make([]*pb.WlanChannelStat, 0, len(channelStats))
	for _, ch := range channelStats {
		pbChannels = append(pbChannels, &pb.WlanChannelStat{
			Channel:         ch.Channel,
			FreqMhz:         ch.FreqMHz,
			ActiveMs:        ch.ActiveMs,
			BusyMs:          ch.BusyMs,
			UtilizationPct:  ch.UtilizationPct,
			Frames:          ch.Frames,
		})
	}

	pbNetworks := make([]*pb.WlanNetwork, 0, len(networks))
	for _, n := range networks {
		pbNetworks = append(pbNetworks, &pb.WlanNetwork{
			Bssid:   n.BSSID,
			Ssid:    n.SSID,
			Channel: n.Channel,
			FreqMhz: n.FreqMHz,
			RssiDbm: n.RSSIdBm,
			Beacons: n.Beacons,
		})
	}

	a.Logger.Info("WLAN sense done",
		slog.String("test", spec.Name), slog.String("interface", usedIface),
		slog.Int("stations", len(pbStations)), slog.Int("channels", len(pbChannels)),
		slog.Int("networks", len(pbNetworks)), slog.Uint64("sweepMs", uint64(sweepMs)))
	result.Result = &pb.TestResult_WlanSense{WlanSense: &pb.WlanSenseResult{
		Interface: usedIface,
		Stations:  pbStations,
		Channels:  pbChannels,
		Networks:  pbNetworks,
		SweepMs:   sweepMs,
		Demo:      probe.DemoModeWlanSense(),
	}}
	sendResult(ctx, results, result)
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
