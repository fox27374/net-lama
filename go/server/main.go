package main

import (
	"io"
	"log"
	"net"
	"time"

	pb "net-lama/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type server struct {
	pb.UnimplementedControlServiceServer
}

// ControlStream implements bidirectional streaming with client identification
func (s *server) ControlStream(stream pb.ControlService_ControlStreamServer) error {
	ctx := stream.Context()

	// --- Extract client metadata ---
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	clientID := "unknown"
	if ids := md.Get("client-id"); len(ids) > 0 {
		clientID = ids[0]
	}

	clientVersion := "unknown"
	if versions := md.Get("client-version"); len(versions) > 0 {
		clientVersion = versions[0]
	}

	if clientID == "unknown" {
		return status.Error(codes.Unauthenticated, "client-id required")
	}

	log.Printf(
		"[CONNECT] client_id=%s version=%s",
		clientID,
		clientVersion,
	)

	errCh := make(chan error, 2)

	// --- Receive loop (metrics from client) ---
	go func() {
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				errCh <- nil
				return
			}
			if err != nil {
				errCh <- err
				return
			}

			if metrics := msg.GetMetrics(); metrics != nil {
				log.Printf(
					"[METRICS] client_id=%s upload=%.2fMbps download=%.2fMbps latency=%.2fms",
					clientID,
					metrics.UploadMbps,
					metrics.DownloadMbps,
					metrics.LatencyMs,
				)
			}
		}
	}()

	// --- Send loop (commands to client) ---
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cmd := &pb.ControlMessage{
					Payload: &pb.ControlMessage_Command{
						Command: &pb.Command{
							Type: "get_netdata",
						},
					},
				}

				log.Printf("[COMMAND] client_id=%s -> get_netdata", clientID)

				if err := stream.Send(cmd); err != nil {
					errCh <- err
					return
				}

			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}
	}()

	// --- Wait for termination ---
	err := <-errCh
	log.Printf("[DISCONNECT] client_id=%s error=%v", clientID, err)
	return err
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterControlServiceServer(grpcServer, &server{})

	log.Println("gRPC server listening on :50051")
	log.Fatal(grpcServer.Serve(lis))
}
