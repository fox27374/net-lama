package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fox27374/net-lama/internal/agent"
)

const version = "0.1.0"

func main() {
	serverAddr := flag.String("server", envOr("NETLAMA_SERVER", "localhost:50051"), "Address of the net-lama server (host:port)")
	clientID := flag.String("id", envOr("NETLAMA_CLIENT_ID", ""), "Client ID for logging (defaults to the hostname)")
	token := flag.String("token", envOr("NETLAMA_TOKEN", ""), "Agent token issued by the server")
	flag.Parse()

	logger := newLogger()

	if *token == "" {
		logger.Error("An agent token is required, set -token or NETLAMA_TOKEN")
		os.Exit(1)
	}

	if *clientID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			logger.Error("Determining hostname failed, set -id or NETLAMA_CLIENT_ID", slog.Any("error", err))
			os.Exit(1)
		}
		*clientID = hostname
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := &agent.Agent{
		ServerAddr: *serverAddr,
		ClientID:   *clientID,
		Token:      *token,
		Version:    version,
		Logger:     logger,
	}

	logger.Info("Agent starting",
		slog.String("server", *serverAddr),
		slog.String("clientId", *clientID),
		slog.String("version", version),
	)

	if err := a.Run(ctx); err != nil {
		logger.Error("Agent failed", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("Agent exited")
}

func envOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func newLogger() *slog.Logger {
	logLevel := slog.LevelInfo
	addSource := false
	if _, ok := os.LookupEnv("DEBUG"); ok {
		logLevel = slog.LevelDebug
		addSource = true
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: addSource,
	}))
	slog.SetDefault(logger)
	return logger
}
