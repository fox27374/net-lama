package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	"github.com/fox27374/net-lama/internal/api"
	"github.com/fox27374/net-lama/internal/server"
	"github.com/fox27374/net-lama/internal/store"
	"github.com/fox27374/net-lama/internal/web"
	pb "github.com/fox27374/net-lama/proto"
)

func main() {
	grpcAddr := flag.String("grpc", envOr("NETLAMA_GRPC_ADDR", ":50051"), "gRPC listen address for agents")
	httpAddr := flag.String("http", envOr("NETLAMA_HTTP_ADDR", ":9090"), "HTTP listen address for UI, API and metrics")
	dbPath := flag.String("db", envOr("NETLAMA_DB", "netlama.db"), "Path to the SQLite database")
	flag.Parse()

	logger := newLogger()

	st, err := store.Open(*dbPath)
	if err != nil {
		logger.Error("Opening database failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer st.Close()

	if err := bootstrapAdmin(st, logger); err != nil {
		logger.Error("Bootstrapping admin user failed", slog.Any("error", err))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	registry := prometheus.NewRegistry()
	metrics := server.NewMetrics(registry)
	srv := server.New(st, metrics, logger)

	// HTTP: web UI, JSON API and Prometheus metrics
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	api.New(st, srv, logger).Register(mux)
	mux.Handle("/", web.Handler())
	httpServer := &http.Server{Addr: *httpAddr, Handler: mux}

	errChan := make(chan error, 2)

	go func() {
		logger.Info("Starting HTTP server (UI, API, metrics)", slog.String("addr", *httpAddr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// gRPC control server
	listener, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		logger.Error("Listening failed", slog.Any("error", err))
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterControlServiceServer(grpcServer, srv)

	go func() {
		logger.Info("Starting gRPC control server", slog.String("addr", *grpcAddr))
		if err := grpcServer.Serve(listener); err != nil {
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		logger.Error("Server failed", slog.Any("error", err))
	case <-ctx.Done():
		logger.Info("Shutting down...")
	}

	// GracefulStop blocks as long as control streams are open,
	// so force-stop after a timeout to cut the long-lived agent streams.
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		grpcServer.Stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown failed", slog.Any("error", err))
	}

	logger.Info("Server exited")
}

// bootstrapAdmin creates the initial admin user on an empty database.
// The password comes from NETLAMA_ADMIN_PASSWORD or is generated.
func bootstrapAdmin(st *store.Store, logger *slog.Logger) error {
	count, err := st.CountUsers()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	password := os.Getenv("NETLAMA_ADMIN_PASSWORD")
	generated := false
	if password == "" {
		password = store.NewToken()[:16]
		generated = true
	}

	if _, err := st.CreateUser("", "admin", password, true); err != nil {
		return err
	}

	if generated {
		logger.Info("Created initial admin user",
			slog.String("username", "admin"),
			slog.String("password", password),
			slog.String("note", "set NETLAMA_ADMIN_PASSWORD to control this; change after first login"),
		)
	} else {
		logger.Info("Created initial admin user from NETLAMA_ADMIN_PASSWORD",
			slog.String("username", "admin"))
	}
	return nil
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
