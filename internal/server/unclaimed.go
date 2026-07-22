package server

import (
	"encoding/json"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fox27374/net-lama/internal/store"
	pb "github.com/fox27374/net-lama/proto"
)

// handleUnclaimedRegister records (or refreshes) a pending enrollment for a
// device that presented a tenant's enrollment token instead of a real
// per-agent one. There is no site yet and nothing to run, so it never joins
// the connected-agent registry and never gets a Config push — the stream
// ends immediately, and the agent's own reconnect/backoff loop becomes the
// enrollment heartbeat (each retry refreshes last_seen).
func (s *Server) handleUnclaimedRegister(tenant *store.Tenant, register *pb.Register) error {
	clientID := register.ClientId
	if clientID == "" {
		clientID = "unknown"
	}

	var caps json.RawMessage
	if c := register.Capabilities; len(c) > 0 && !isLegacyCapabilities(c) {
		if data, err := json.Marshal(c); err == nil {
			caps = data
		}
	}
	ifaces := marshalNetworkInterfaces(register.NetworkInterfaces)

	if err := s.Store.UpsertUnclaimedAgent(tenant.ID, clientID, register.Version, caps, ifaces); err != nil {
		s.Logger.Warn("Recording unclaimed agent failed", slog.Any("error", err))
	} else {
		s.Logger.Info("Unclaimed agent seen",
			slog.String("tenant", tenant.Name),
			slog.String("clientId", clientID),
		)
	}

	return status.Error(codes.FailedPrecondition, "agent not yet claimed; waiting for site assignment in the UI")
}
