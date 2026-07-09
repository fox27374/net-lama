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
	useTLS := flag.Bool("tls", envEnabled("NETLAMA_TLS"), "Connect to the server over TLS")
	tlsCA := flag.String("tls-ca", envOr("NETLAMA_TLS_CA", ""), "PEM file of the CA/server cert to trust (else system roots)")
	tlsInsecure := flag.Bool("tls-insecure", envEnabled("NETLAMA_TLS_INSECURE"), "Skip server certificate verification (still encrypted)")
	tlsCert := flag.String("tls-cert", envOr("NETLAMA_TLS_CERT", ""), "Client certificate for mTLS (issued per agent by the server)")
	tlsKey := flag.String("tls-key", envOr("NETLAMA_TLS_KEY", ""), "Key for the mTLS client certificate")
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
		ServerAddr:  *serverAddr,
		ClientID:    *clientID,
		Token:       *token,
		Version:     version,
		Logger:      logger,
		TLS:         *useTLS,
		TLSCAFile:   *tlsCA,
		TLSInsecure: *tlsInsecure,
		TLSCertFile: *tlsCert,
		TLSKeyFile:  *tlsKey,
	}

	logger.Info("Agent starting",
		slog.String("server", *serverAddr),
		slog.String("clientId", *clientID),
		slog.String("version", version),
		slog.Bool("tls", *useTLS),
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

// envEnabled treats unset/""/0/false/off/no as disabled.
func envEnabled(key string) bool {
	switch os.Getenv(key) {
	case "", "0", "false", "off", "no":
		return false
	default:
		return true
	}
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
