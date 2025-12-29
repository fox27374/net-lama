package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
)

const (
	pingCount = 3
)

var (
	debug   = false
	logger  *slog.Logger
	netData NetData
)

type ByteRate float64

type Speed struct {
	Speed float64 `json:"speed"`
	Unit  string  `json:"unit"`
}

type App struct {
	dataChan chan<- NetData
	errChan  chan<- error
}

func init() {
	// Check if the "DEBUG" environment variable is set
	_, ok := os.LookupEnv("DEBUG")
	if ok {
		debug = true
	}

	var logLevel slog.Level
	var addSource bool

	if debug {
		logLevel = slog.LevelDebug
		addSource = true
	} else {
		logLevel = slog.LevelInfo
		addSource = false
	}

	// Create and configure log handler
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: addSource,
	})

	// Define the global logger variable and
	// set it as default logger
	logger = slog.New(handler)
	slog.SetDefault(logger)
}

func main() {
	logger.Info("Application started")
	dataChan := make(chan NetData)
	errChan := make(chan error)
	quit := make(chan os.Signal, 1)

	go getNetInfo(dataChan, errChan)
	go channelListener(dataChan, errChan, quit)

	// create a type that satisfies the `api.ServerInterface`, which contains an implementation of every operation from the generated code
	server := NewServer()

	r := http.NewServeMux()

	// get an `http.Handler` that we can use
	h := HandlerFromMux(server, r)

	s := &http.Server{
		Handler: h,
		Addr:    "0.0.0.0:8080",
	}

	// And we serve HTTP until the world ends.
	log.Fatal(s.ListenAndServe())
}

func channelListener(d <-chan NetData, e <-chan error, q <-chan os.Signal) {
	for {
		select {
		case data := <-d:
			netData = data
			logger.Info("SpeedTest Result",
				slog.String("server_name", *data.Name),
				slog.String("country", *data.Country),
				slog.Float64("latency", *data.Latency),
				slog.Float64("dl_speed_mbps", *data.Dlspeed),
				slog.Float64("ul_speed_mbps", *data.Ulspeed),
				slog.String("user_ip", *data.Userip),
				slog.String("user_isp", *data.Userisp),
			)
		case err := <-e:
			fmt.Println(err)
		case <-q:
			slog.Info("Received shutdown signal, stopping...")
			return
		}
	}
}
