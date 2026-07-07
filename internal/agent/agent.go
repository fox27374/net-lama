package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/fox27374/net-lama/proto"
)

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
}

// Run connects to the server and keeps the control stream alive,
// reconnecting with exponential backoff until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
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
	conn, err := grpc.NewClient(
		a.ServerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
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

	register := &pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.Register{
				ClientId:     a.ClientID,
				ClientType:   "networktest",
				Version:      a.Version,
				Capabilities: []string{"speedtest", "ping", "dns"},
				Token:        a.Token,
			},
		},
	}
	if err := stream.Send(register); err != nil {
		return fmt.Errorf("sending register: %w", err)
	}
	a.Logger.Info("Registered with server",
		slog.String("server", a.ServerAddr),
		slog.String("clientId", a.ClientID),
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

	// Send loop: single writer on the stream
	for {
		select {
		case result := <-results:
			msg := &pb.AgentMessage{Payload: &pb.AgentMessage_Result{Result: result}}
			if err := stream.Send(msg); err != nil {
				return fmt.Errorf("sending result: %w", err)
			}
		case err := <-recvErr:
			return fmt.Errorf("receiving: %w", err)
		case <-streamCtx.Done():
			return streamCtx.Err()
		}
	}
}
