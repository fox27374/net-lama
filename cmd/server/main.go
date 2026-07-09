package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

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
	issueAgentCert := flag.String("issue-agent-cert", "", "Issue an mTLS client certificate for the named agent (signed by the built-in agent CA) and exit")
	flag.Parse()

	logger := newLogger()

	if *issueAgentCert != "" {
		certPath, keyPath, err := server.IssueAgentCert(filepath.Dir(*dbPath), *issueAgentCert)
		if err != nil {
			logger.Error("Issuing agent certificate failed", slog.Any("error", err))
			os.Exit(1)
		}
		logger.Info("Issued agent client certificate",
			slog.String("agent", *issueAgentCert),
			slog.String("cert", certPath),
			slog.String("key", keyPath),
			slog.String("note", "copy both to the agent and set NETLAMA_TLS_CERT/KEY there"))
		return
	}

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

	// TLS for both the gRPC control stream and the HTTP UI/API.
	tlsHosts := []string{"localhost", "127.0.0.1"}
	if h := os.Getenv("NETLAMA_TLS_HOSTS"); h != "" {
		tlsHosts = strings.Split(h, ",")
	}
	selfSigned := envEnabled("NETLAMA_TLS_SELF_SIGNED")
	tlsConfig, err := server.LoadTLSConfig(
		os.Getenv("NETLAMA_TLS_CERT"), os.Getenv("NETLAMA_TLS_KEY"),
		selfSigned, tlsHosts, filepath.Dir(*dbPath))
	if err != nil {
		logger.Error("Loading TLS config failed", slog.Any("error", err))
		os.Exit(1)
	}
	tlsOn := tlsConfig != nil
	if tlsOn {
		logger.Info("TLS enabled for gRPC and HTTP")
		if selfSigned && os.Getenv("NETLAMA_TLS_CERT") == "" {
			logger.Info("Using self-signed certificate",
				slog.String("path", server.SelfSignedCertPath(filepath.Dir(*dbPath))))
		}
	} else {
		logger.Warn("TLS is OFF — the agent control stream and web UI are unencrypted; set NETLAMA_TLS_SELF_SIGNED=1 or provide NETLAMA_TLS_CERT/KEY")
	}

	// mTLS: agents must present a client cert on the gRPC stream. The HTTP
	// UI/API keeps plain server-auth TLS (browsers have no client certs).
	mtlsCA := os.Getenv("NETLAMA_MTLS_CA")
	mtlsOn := envEnabled("NETLAMA_MTLS") || mtlsCA != ""
	grpcTLS := tlsConfig
	if mtlsOn {
		if !tlsOn {
			logger.Error("mTLS requires TLS: set NETLAMA_TLS_SELF_SIGNED=1 or provide NETLAMA_TLS_CERT/KEY")
			os.Exit(1)
		}
		pool, err := server.ClientCAPool(mtlsCA, filepath.Dir(*dbPath))
		if err != nil {
			logger.Error("Loading mTLS client CA failed", slog.Any("error", err))
			os.Exit(1)
		}
		grpcTLS = tlsConfig.Clone()
		grpcTLS.ClientCAs = pool
		grpcTLS.ClientAuth = tls.RequireAndVerifyClientCert
		if mtlsCA != "" {
			logger.Info("mTLS enabled for the agent control stream", slog.String("clientCA", mtlsCA))
		} else {
			logger.Info("mTLS enabled for the agent control stream",
				slog.String("clientCA", server.AgentCAPath(filepath.Dir(*dbPath))),
				slog.String("note", "issue agent certs with -issue-agent-cert <agent-name>"))
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	registry := prometheus.NewRegistry()
	metrics := server.NewMetrics(registry)
	srv := server.New(st, metrics, logger)
	srv.MTLS = mtlsOn

	// HTTP: web UI, JSON API and Prometheus metrics
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	api.New(st, srv, logger, tlsOn).Register(mux)
	mux.Handle("/", web.Handler())
	httpServer := &http.Server{Addr: *httpAddr, Handler: mux, TLSConfig: tlsConfig}

	errChan := make(chan error, 2)

	go func() {
		scheme := "http"
		if tlsOn {
			scheme = "https"
		}
		logger.Info("Starting HTTP server (UI, API, metrics)", slog.String("addr", *httpAddr), slog.String("scheme", scheme))
		var err error
		if tlsOn {
			err = httpServer.ListenAndServeTLS("", "") // certs come from TLSConfig
		} else {
			err = httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// gRPC control server
	listener, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		logger.Error("Listening failed", slog.Any("error", err))
		os.Exit(1)
	}

	var grpcOpts []grpc.ServerOption
	if tlsOn {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(grpcTLS)))
	}
	grpcServer := grpc.NewServer(grpcOpts...)
	pb.RegisterControlServiceServer(grpcServer, srv)

	go func() {
		logger.Info("Starting gRPC control server", slog.String("addr", *grpcAddr), slog.Bool("tls", tlsOn))
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
