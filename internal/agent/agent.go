package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fox27374/net-lama/internal/logtee"
	"github.com/fox27374/net-lama/internal/probe"
	pb "github.com/fox27374/net-lama/proto"
)

// logShipInterval is how often the send loop drains the buffered Info+
// log lines and ships them to the server, while the stream is connected.
const logShipInterval = 2 * time.Second

// logBufferCapacity bounds how many log lines are held while
// disconnected; the oldest is dropped once full.
const logBufferCapacity = 200

// statsInterval is how often the agent collects and sends resource statistics.
const statsInterval = 30 * time.Second

// transportCredentials builds the gRPC transport security for the agent.
func (a *Agent) transportCredentials() (credentials.TransportCredentials, error) {
	if !a.TLS {
		return insecure.NewCredentials(), nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	switch {
	case a.TLSInsecure:
		cfg.InsecureSkipVerify = true
	case a.TLSCAFile != "":
		pemData, err := os.ReadFile(a.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("reading TLS CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("no certificates found in %s", a.TLSCAFile)
		}
		cfg.RootCAs = pool
	}
	// mTLS: present a client certificate when the server requires one.
	if a.TLSCertFile != "" || a.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(a.TLSCertFile, a.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("loading TLS client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return credentials.NewTLS(cfg), nil
}

const (
	reconnectMinDelay = 1 * time.Second
	reconnectMaxDelay = 30 * time.Second
)

type Agent struct {
	ServerAddr string
	ClientID   string
	Token      string
	Version    string
	Logger     *slog.Logger

	// TLS transport security to the server.
	TLS         bool   // use TLS
	TLSCAFile   string // PEM of the CA/server cert to trust (else system roots)
	TLSInsecure bool   // skip server cert verification (still encrypted)
	TLSCertFile string // client certificate for mTLS (issued per agent)
	TLSKeyFile  string // key for TLSCertFile

	// WlanIface overrides the monitor-capable interface for wlan_passive tests;
	// empty = auto-pick the first monitor-capable interface.
	WlanIface string

	// logBuf holds Info+ log lines (Logger tees into it) until the send
	// loop can ship them to the server; it survives across reconnects so
	// nothing logged while disconnected is lost (up to its capacity).
	logBuf *logRingBuffer

	// statsCollector gathers CPU, memory, and disk statistics.
	statsCollector *probe.StatsCollector

	// wlanMu serializes access to the monitor interface and wlan state
	wlanMu    sync.Mutex
	wlanState map[string]*wlanPassiveState // per test ID
}

type wlanPassiveState struct {
	InterestingChannels []uint32
}

// Run connects to the server and keeps the control stream alive,
// reconnecting with exponential backoff until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	if a.logBuf == nil {
		a.logBuf = newLogRingBuffer(logBufferCapacity)
		// Tee Info+ log lines into the buffer; the chatty per-connection
		// debug lines never reach it since the tee only forwards Info+.
		a.Logger = slog.New(logtee.New(a.Logger.Handler(), func(e logtee.Entry) {
			a.logBuf.Push(e)
		}))
	}

	if a.statsCollector == nil {
		a.statsCollector = probe.NewStatsCollector()
	}

	delay := reconnectMinDelay

	for {
		start := time.Now()
		err := a.runStream(ctx)
		if ctx.Err() != nil {
			return nil
		}

		// Reset backoff if the connection was up for a while
		if time.Since(start) > time.Minute {
			delay = reconnectMinDelay
		}

		a.Logger.Warn("Connection lost, reconnecting",
			slog.Any("error", err),
			slog.Duration("delay", delay),
		)

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil
		}

		delay *= 2
		if delay > reconnectMaxDelay {
			delay = reconnectMaxDelay
		}
	}
}

// runStream opens one control stream: registers, then processes incoming
// config/commands and sends back test results until the stream breaks.
func (a *Agent) runStream(ctx context.Context) error {
	creds, err := a.transportCredentials()
	if err != nil {
		return err
	}
	conn, err := grpc.NewClient(a.ServerAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return fmt.Errorf("creating connection: %w", err)
	}
	defer conn.Close()

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := pb.NewControlServiceClient(conn).ControlStream(streamCtx)
	if err != nil {
		return fmt.Errorf("opening stream: %w", err)
	}

	detectedIfaces := probe.WirelessInterfaces(streamCtx)
	var wifaces []*pb.WirelessInterface
	for _, iface := range detectedIfaces {
		wifaces = append(wifaces, &pb.WirelessInterface{
			Name:            iface.Name,
			Phy:             iface.PHY,
			SupportsMonitor: iface.SupportsMonitor,
		})
	}

	capabilities := probe.DetectCapabilities(len(wifaces) > 0, detectedIfaces)

	register := &pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.Register{
				ClientId:           a.ClientID,
				ClientType:         "networktest",
				Version:            a.Version,
				Capabilities:       capabilities,
				Token:              a.Token,
				WirelessInterfaces: wifaces,
			},
		},
	}
	if err := stream.Send(register); err != nil {
		return fmt.Errorf("sending register: %w", err)
	}
	a.Logger.Info("Registered with server",
		slog.String("server", a.ServerAddr),
		slog.String("clientId", a.ClientID),
		slog.Any("capabilities", capabilities),
	)

	cfgCh := make(chan *pb.Config, 1)
	cmdCh := make(chan *pb.Command, 4)
	results := make(chan *pb.TestResult, 16)
	recvErr := make(chan error, 1)

	// Receive loop: config and commands from the server
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				recvErr <- err
				cancel()
				return
			}
			switch payload := msg.Payload.(type) {
			case *pb.ServerMessage_Config:
				a.Logger.Info("Received config from server")
				cfgCh <- payload.Config
			case *pb.ServerMessage_Command:
				a.Logger.Info("Received command", slog.String("type", payload.Command.Type.String()))
				cmdCh <- payload.Command
			}
		}
	}()

	// Scheduler: runs the tests according to the active config
	go a.schedule(streamCtx, cfgCh, cmdCh, results)

	// Ship any log lines buffered while disconnected right away, then
	// keep draining periodically.
	a.sendBufferedLogs(stream)
	logTicker := time.NewTicker(logShipInterval)
	defer logTicker.Stop()

	// Send stats every 30 seconds
	statsTicker := time.NewTicker(statsInterval)
	defer statsTicker.Stop()
	// Send stats shortly after registration
	a.sendStats(stream)

	// Send loop: single writer on the stream — results, log lines, and
	// stats all go through stream.Send here so they can never interleave.
	for {
		select {
		case result := <-results:
			msg := &pb.AgentMessage{Payload: &pb.AgentMessage_Result{Result: result}}
			if err := stream.Send(msg); err != nil {
				return fmt.Errorf("sending result: %w", err)
			}
		case <-logTicker.C:
			if err := a.sendBufferedLogs(stream); err != nil {
				return err
			}
		case <-statsTicker.C:
			a.sendStats(stream)
		case err := <-recvErr:
			return fmt.Errorf("receiving: %w", err)
		case <-streamCtx.Done():
			return streamCtx.Err()
		}
	}
}

// sendBufferedLogs drains the log ring buffer and ships every entry on
// the stream. It is only ever called from the single-writer send loop
// (plus once right after registration), so it cannot interleave with
// result sends.
func (a *Agent) sendBufferedLogs(stream pb.ControlService_ControlStreamClient) error {
	for _, e := range a.logBuf.Drain() {
		msg := &pb.AgentMessage{Payload: &pb.AgentMessage_Log{Log: &pb.LogEntry{
			Time:    timestamppb.New(e.Time),
			Level:   e.Level,
			Message: e.Message,
			Scope:   e.Scope,
		}}}
		if err := stream.Send(msg); err != nil {
			return fmt.Errorf("sending log: %w", err)
		}
	}
	return nil
}

// sendStats collects and sends resource statistics on the stream.
// It is only ever called from the single-writer send loop, so it
// cannot interleave with result or log sends.
func (a *Agent) sendStats(stream pb.ControlService_ControlStreamClient) {
	cpu, memUsed, memTotal, diskUsed, diskTotal, agentCpu, agentMem, pidCount, uptime, ok, err := a.statsCollector.Collect()
	if err != nil {
		a.Logger.Debug("Failed to collect stats", slog.Any("error", err))
		return
	}
	if !ok {
		// Stats not available on this platform (e.g., not Linux)
		return
	}

	msg := &pb.AgentMessage{
		Payload: &pb.AgentMessage_Stats{
			Stats: &pb.AgentStats{
				Time:            timestamppb.New(time.Now()),
				CpuPercent:      cpu,
				MemUsedBytes:    memUsed,
				MemTotalBytes:   memTotal,
				DiskUsedBytes:   diskUsed,
				DiskTotalBytes:  diskTotal,
				AgentCpuPercent: agentCpu,
				AgentMemBytes:   agentMem,
				PidCount:        pidCount,
				UptimeSeconds:   uptime,
			},
		},
	}
	if err := stream.Send(msg); err != nil {
		a.Logger.Debug("Failed to send stats", slog.Any("error", err))
		return
	}
}
