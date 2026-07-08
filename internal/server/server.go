package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/fox27374/net-lama/internal/store"
	pb "github.com/fox27374/net-lama/proto"
)

type connectedAgent struct {
	agent  *store.Agent
	tenant string
	push   chan *pb.ServerMessage
}

type Server struct {
	pb.UnimplementedControlServiceServer

	Store   *store.Store
	Metrics *Metrics
	Logger  *slog.Logger

	mu        sync.Mutex
	connected map[string]*connectedAgent // keyed by agent ID

	breachMu    sync.Mutex
	breachCount map[string]int // consecutive alert-rule breaches, keyed by rule|agent
}

func New(st *store.Store, metrics *Metrics, logger *slog.Logger) *Server {
	return &Server{
		Store:       st,
		Metrics:     metrics,
		Logger:      logger,
		connected:   make(map[string]*connectedAgent),
		breachCount: make(map[string]int),
	}
}

// ConfigForAgent builds the protobuf config from the tests assigned to
// the agent's site.
func (s *Server) ConfigForAgent(agent *store.Agent) (*pb.Config, error) {
	tests, err := s.Store.TestsForSite(agent.SiteID)
	if err != nil {
		return nil, fmt.Errorf("loading tests for site: %w", err)
	}

	cfg := &pb.Config{WlanInterface: agent.WlanInterface}
	for _, t := range tests {
		spec, err := TestSpec(t)
		if err != nil {
			s.Logger.Warn("Skipping invalid test definition",
				slog.String("test", t.Name), slog.Any("error", err))
			continue
		}
		cfg.Tests = append(cfg.Tests, spec)
	}
	return cfg, nil
}

// ControlStream handles one connected agent for the lifetime of its stream.
func (s *Server) ControlStream(stream pb.ControlService_ControlStreamServer) error {
	// The first message must be a registration with a valid token
	first, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("receiving registration: %w", err)
	}
	register := first.GetRegister()
	if register == nil || register.Token == "" {
		return status.Error(codes.Unauthenticated, "first message must be a register message with a token")
	}

	agent, err := s.Store.GetAgentByToken(register.Token)
	if err != nil {
		s.Logger.Warn("Agent with invalid token rejected", slog.String("clientId", register.ClientId))
		return status.Error(codes.Unauthenticated, "invalid agent token")
	}

	// Record the wireless interfaces the agent reported, so the UI can
	// offer them for selection.
	if ifaces := register.WirelessInterfaces; ifaces != nil {
		type wi struct {
			Name            string `json:"name"`
			PHY             string `json:"phy"`
			SupportsMonitor bool   `json:"supportsMonitor"`
		}
		list := make([]wi, 0, len(ifaces))
		for _, w := range ifaces {
			list = append(list, wi{Name: w.Name, PHY: w.Phy, SupportsMonitor: w.SupportsMonitor})
		}
		if data, err := json.Marshal(list); err == nil {
			if err := s.Store.SetAgentInterfaces(agent.ID, data); err != nil {
				s.Logger.Warn("Storing agent interfaces failed", slog.Any("error", err))
			}
			agent.WirelessInterfaces = data
		}
	}

	tenantName := agent.TenantID
	if tenants, err := s.Store.ListTenants(); err == nil {
		for _, t := range tenants {
			if t.ID == agent.TenantID {
				tenantName = t.Name
				break
			}
		}
	}

	conn := &connectedAgent{
		agent:  agent,
		tenant: tenantName,
		push:   make(chan *pb.ServerMessage, 4),
	}

	s.mu.Lock()
	if old, ok := s.connected[agent.ID]; ok {
		close(old.push)
	}
	s.connected[agent.ID] = conn
	s.mu.Unlock()
	s.Metrics.SetConnected(tenantName, agent.SiteName, agent.Name, true)

	logger := s.Logger.With(
		slog.String("agent", agent.Name),
		slog.String("site", agent.SiteName),
		slog.String("tenant", tenantName),
	)
	logger.Info("Agent connected",
		slog.String("clientId", register.ClientId),
		slog.String("version", register.Version),
	)

	defer func() {
		s.mu.Lock()
		if s.connected[agent.ID] == conn {
			delete(s.connected, agent.ID)
		}
		s.mu.Unlock()
		s.Metrics.SetConnected(tenantName, agent.SiteName, agent.Name, false)
		logger.Info("Agent disconnected")
	}()

	// Push the agent's configuration
	cfg, err := s.ConfigForAgent(agent)
	if err != nil {
		return err
	}
	if err := stream.Send(&pb.ServerMessage{Payload: &pb.ServerMessage_Config{Config: cfg}}); err != nil {
		return fmt.Errorf("sending config: %w", err)
	}
	logger.Info("Config sent to agent", slog.Int("tests", len(cfg.Tests)))

	// Receive loop feeding a channel so we can select on pushes as well
	recvCh := make(chan *pb.AgentMessage)
	recvErr := make(chan error, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				recvErr <- err
				return
			}
			select {
			case recvCh <- msg:
			case <-stream.Context().Done():
				return
			}
		}
	}()

	for {
		select {
		case msg := <-recvCh:
			switch payload := msg.Payload.(type) {
			case *pb.AgentMessage_Result:
				s.handleResult(logger, conn, payload.Result)
			case *pb.AgentMessage_Log:
				logger.Info("Agent log",
					slog.String("level", payload.Log.Level),
					slog.String("message", payload.Log.Message),
				)
			}

		case push, ok := <-conn.push:
			if !ok {
				return status.Error(codes.Aborted, "replaced by a new connection for the same agent")
			}
			if err := stream.Send(push); err != nil {
				return fmt.Errorf("sending push message: %w", err)
			}
			logger.Info("Pushed message to agent")

		case err := <-recvErr:
			if err == io.EOF {
				return nil
			}
			return err

		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// PushConfigs rebuilds and pushes the configuration for every listed
// agent that is currently connected. Returns the number of pushes.
func (s *Server) PushConfigs(agentIDs []string) int {
	pushed := 0
	for _, id := range agentIDs {
		s.mu.Lock()
		conn, ok := s.connected[id]
		s.mu.Unlock()
		if !ok {
			continue
		}

		cfg, err := s.ConfigForAgent(conn.agent)
		if err != nil {
			s.Logger.Error("Building config for push failed",
				slog.String("agent", conn.agent.Name), slog.Any("error", err))
			continue
		}

		msg := &pb.ServerMessage{Payload: &pb.ServerMessage_Config{Config: cfg}}
		select {
		case conn.push <- msg:
			pushed++
		default:
		}
	}
	return pushed
}

// RefreshAgent reloads a connected agent's record from the store (e.g.
// after a rename or site move), fixes the connection metrics labels and
// pushes the agent's current config. Returns the number of pushes (0 or 1).
func (s *Server) RefreshAgent(agentID string) int {
	s.mu.Lock()
	conn, ok := s.connected[agentID]
	s.mu.Unlock()
	if !ok {
		return 0
	}

	fresh, err := s.Store.GetAgent(agentID)
	if err != nil {
		return 0
	}

	if fresh.SiteName != conn.agent.SiteName || fresh.Name != conn.agent.Name {
		s.Metrics.SetConnected(conn.tenant, conn.agent.SiteName, conn.agent.Name, false)
		s.Metrics.SetConnected(conn.tenant, fresh.SiteName, fresh.Name, true)
	}
	conn.agent = fresh

	return s.PushConfigs([]string{agentID})
}

// RunTest asks a connected agent to run a specific test immediately.
// Returns true if the command was queued to a connected agent.
func (s *Server) RunTest(agentID, testID string) bool {
	s.mu.Lock()
	conn, ok := s.connected[agentID]
	s.mu.Unlock()
	if !ok {
		return false
	}
	msg := &pb.ServerMessage{Payload: &pb.ServerMessage_Command{
		Command: &pb.Command{Type: pb.Command_RUN_TEST, TestId: testID},
	}}
	select {
	case conn.push <- msg:
		return true
	default:
		return false
	}
}

// AgentConnected reports whether an agent currently has an open stream.
func (s *Server) AgentConnected(agentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.connected[agentID]
	return ok
}

// resultOK decides whether a result counts as healthy for the overview.
func resultOK(r *pb.TestResult) bool {
	if r.Error != "" {
		return false
	}
	switch v := r.Result.(type) {
	case *pb.TestResult_Ping:
		return v.Ping.PacketsReceived > 0
	case *pb.TestResult_Dns:
		return v.Dns.Success
	case *pb.TestResult_Http:
		return v.Http.StatusCode >= 200 && v.Http.StatusCode < 400
	case *pb.TestResult_Tcp:
		return v.Tcp.Connected
	case *pb.TestResult_WlanScan:
		return len(v.WlanScan.AccessPoints) > 0
	case *pb.TestResult_Traceroute:
		return v.Traceroute.Reached
	case *pb.TestResult_Speedtest:
		return true
	}
	return true
}

func (s *Server) handleResult(logger *slog.Logger, conn *connectedAgent, result *pb.TestResult) {
	s.Metrics.Record(conn.tenant, conn.agent.SiteName, conn.agent.Name, result)

	testType := "unknown"
	switch result.Result.(type) {
	case *pb.TestResult_Speedtest:
		testType = "speedtest"
	case *pb.TestResult_Ping:
		testType = "ping"
	case *pb.TestResult_Dns:
		testType = "dns"
	case *pb.TestResult_Http:
		testType = "http"
	case *pb.TestResult_Tcp:
		testType = "tcp"
	case *pb.TestResult_WlanScan:
		testType = "wlan_scan"
	case *pb.TestResult_Traceroute:
		testType = "traceroute"
	}

	// Persist the result. EmitDefaultValues keeps zero-valued fields
	// (reached=false, loss=0, ...) in the JSON so the UI can rely on them.
	payload, err := protojson.MarshalOptions{EmitDefaultValues: true}.Marshal(result)
	if err != nil {
		logger.Error("Marshalling result failed", slog.Any("error", err))
		return
	}
	t := time.Now()
	if result.Time != nil {
		t = result.Time.AsTime()
	}
	err = s.Store.AddResult(&store.Result{
		AgentID:  conn.agent.ID,
		TestID:   result.TestId,
		TestName: result.TestName,
		TestType: testType,
		Time:     t,
		Error:    result.Error,
		OK:       resultOK(result),
		Payload:  json.RawMessage(payload),
	})
	if err != nil {
		logger.Error("Storing result failed", slog.Any("error", err))
	}

	// Evaluate alert rules against this result.
	s.evaluateAlerts(conn, result)

	if result.Error != "" {
		logger.Warn("Test failed on agent",
			slog.String("test", result.TestName),
			slog.String("error", result.Error),
		)
		return
	}
	logger.Info("Result received", slog.String("test", result.TestName), slog.String("type", testType))
}
