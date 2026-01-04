package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	testIntervallSeconds = 60
)

type APIConfig struct {
	Addr    string
	Port    int
	Timeout int
}

type metrics struct {
	cpuTemp    prometheus.Gauge
	hdFailures *prometheus.CounterVec
}

var (
	debug   = false
	logger  *slog.Logger
	netData NetData
)

func main() {
	logger.Info("Application started")

	apiCfg := APIConfig{
		Addr:    "0.0.0.0",
		Port:    8080,
		Timeout: 5,
	}

	// Create a context that is cancelled when the OS sends an interrupt signal
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()

	dataChan := make(chan NetData)

	// Start the data channel listener
	logger.Info("Starting datachannel listener")
	go channelListener(ctx, dataChan)

	// 2. Start the Ticker Routine
	go func() {
		ticker := time.NewTicker(testIntervallSeconds * time.Second)
		defer ticker.Stop()

		logger.Info(fmt.Sprintf("Starting Speedtests with %d second intervals", testIntervallSeconds))
		getNetInfo(ctx, dataChan)

		for {
			select {
			case <-ticker.C:
				getNetInfo(ctx, dataChan)
			case <-ctx.Done():
				logger.Info("Ticker routine stopping...")
				return
			}
		}
	}()

	// Start API server
	logger.Info("Starting API server")
	err := webServer(ctx, apiCfg)
	if err != nil {
		logger.Error("Application failed", slog.Any("error", err))
	}

}

func channelListener(ctx context.Context, d <-chan NetData) {
	for {
		select {
		case data := <-d:
			if data.Error != nil {
				netData = NetData{}
				logger.Info("SpeedTest Error received")
				logger.Error(*data.Errormsg)
			} else {
				netData = data
				mDlSpeed.Set(float64(*data.Dlspeed))
				mUlSpeed.Set(float64(*data.Ulspeed))
				logger.Info("SpeedTest Result received")
			}
		case <-ctx.Done():
			logger.Info("Channel listener stopping...")
			return
		}
	}
}

func webServer(ctx context.Context, cfg APIConfig) error {
	// server := NewServer()
	// r := http.NewServeMux()
	// r.Handle("/metrics", promhttp.Handler())
	// h := HandlerFromMux(server, r)

	// s := &http.Server{
	// 	Handler: h,
	// 	Addr:    fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port),
	// }

	// OpenAPI implementation
	apiImpl := NewServer()

	// Custom Prometheus registry (no defaults)
	registry := prometheus.NewRegistry()
	registerMetrics(registry)

	// HTTP mux
	mux := http.NewServeMux()

	// Metrics endpoint (ONLY your metrics)
	mux.Handle(
		"/metrics",
		promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
	)

	// OpenAPI handler
	apiHandler := HandlerFromMux(apiImpl, mux)

	s := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port),
		Handler: apiHandler,
	}

	// Channel to capture errors from the goroutine
	errChan := make(chan error, 1)

	go func() {
		logger.Info("Starting server", slog.String("addr", s.Addr))
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for either an error or the shutdown signal
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		logger.Info("Shutting down API server...")
	}

	// Graceful shutdown logic
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Second)
	defer cancel()

	if err := s.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	logger.Info("Server exiting")
	return nil
}

func NewMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		cpuTemp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cpu_temperature_celsius",
			Help: "Current temperature of the CPU.",
		}),
		hdFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "hd_errors_total",
				Help: "Number of hard-disk errors.",
			},
			[]string{"device"},
		),
	}
	reg.MustRegister(m.cpuTemp)
	reg.MustRegister(m.hdFailures)
	return m
}
