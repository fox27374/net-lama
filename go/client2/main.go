package main

import (
	"context"
	"log"
	"math/rand"

	pb "net-lama/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func collectNetData() *pb.Metrics {
	return &pb.Metrics{
		UploadMbps:   10 + rand.Float64()*5,
		DownloadMbps: 50 + rand.Float64()*20,
		LatencyMs:    5 + rand.Float64()*10,
	}
}

func main() {
	conn, err := grpc.NewClient(
		"localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := pb.NewControlServiceClient(conn)

	md := metadata.Pairs(
		"client-id", "agent-1234",
		"client-version", "1.0.0",
	)

	ctx := metadata.NewOutgoingContext(context.Background(), md)

	stream, err := client.ControlStream(ctx)
	if err != nil {
		log.Fatal(err)
	}

	errCh := make(chan error)

	// Receive commands from server
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				errCh <- err
				return
			}

			if cmd := msg.GetCommand(); cmd != nil {
				log.Println("Received command:", cmd.Type)

				if cmd.Type == "get_netdata" {
					metrics := collectNetData()

					resp := &pb.ControlMessage{
						Payload: &pb.ControlMessage_Metrics{
							Metrics: metrics,
						},
					}

					if err := stream.Send(resp); err != nil {
						errCh <- err
						return
					}
				}
			}
		}
	}()

	log.Println("Client running")
	log.Fatal(<-errCh)
}
