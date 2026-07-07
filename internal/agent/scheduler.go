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
	}
}

func newResult(spec *pb.TestSpec) *pb.TestResult {
	return &pb.TestResult{
		Time:     timestamppb.Now(),
		TestId:   spec.Id,
		TestName: spec.Name,
	}
}

func (a *Agent) runSpeedtest(ctx context.Context, spec *pb.TestSpec, results chan<- *pb.TestResult) {
	a.Logger.Info("Running speedtest", slog.String("test", spec.Name))
	res, err := probe.Speedtest(ctx)

	result := newResult(spec)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		a.Logger.Error("Speedtest failed", slog.String("test", spec.Name), slog.Any("error", err))
		result.Error = err.Error()
		result.Result = &pb.TestResult_Speedtest{Speedtest: &pb.SpeedtestResult{}}
	} else {
		a.Logger.Info("Speedtest done",
			slog.String("test", spec.Name),
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

func sendResult(ctx context.Context, results chan<- *pb.TestResult, result *pb.TestResult) {
	select {
	case results <- result:
	case <-ctx.Done():
	}
}
