package main

import (
	"log"
	"log/slog"
	"net-lama/api"
	"net/http"
	"os"
)

const (
	pingCount = 3
)

var (
	debug  = false
	logger *slog.Logger
)

type ByteRate float64

// type PLoss float64

type Speed struct {
	Speed float64 `json:"speed"`
	Unit  string  `json:"unit"`
}

// type NetData struct {
// 	Name     string        `json:"name"`
// 	Country  string        `json:"country"`
// 	Distance float64       `json:"distance"`
// 	Latency  time.Duration `json:"latency"`
// 	Jitter   time.Duration `json:"jitter"`
// 	DLSpeed  ByteRate      `json:"dl_speed"`
// 	ULSpeed  ByteRate      `json:"ul_speed"`
// 	// PacketLoss PLoss         `json:"packet_loss"`
// 	UserIP  string `json:"ip"`
// 	UserISP string `json:"isp"`
// }

type App struct {
	dataChan chan<- api.NetData
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
	// dataChan := make(chan api.NetData)
	// errChan := make(chan error)
	// quit := make(chan os.Signal, 1)

	// go channelListener(dataChan, errChan, quit)

	// serverAddr := ":8080"
	// router := setupRoutes(dataChan, errChan)

	// fmt.Printf("Server starting on %s...\n", serverAddr)
	// if err := http.ListenAndServe(serverAddr, router); err != nil {
	// 	fmt.Printf("Error: %s\n", err)
	// }

	// create a type that satisfies the `api.ServerInterface`, which contains an implementation of every operation from the generated code
	server := api.NewServer()

	r := http.NewServeMux()

	// get an `http.Handler` that we can use
	h := api.HandlerFromMux(server, r)

	s := &http.Server{
		Handler: h,
		Addr:    "0.0.0.0:8080",
	}

	// And we serve HTTP until the world ends.
	log.Fatal(s.ListenAndServe())
}

// func channelListener(d <-chan api.NetData, e <-chan error, q <-chan os.Signal) {
// 	for {
// 		select {
// 		case data := <-d:
// 			logger.Info("SpeedTest Result",
// 				slog.String("server_name", data.Name),
// 				slog.String("country", data.Country),
// 				slog.Float64("distance", data.Distance),
// 				slog.Duration("latency", data.Latency),
// 				slog.Duration("jitter", data.Jitter),
// 				slog.Float64("dl_speed_mbps", float64(data.DLSpeed)),
// 				slog.Float64("ul_speed_mbps", float64(data.ULSpeed)),
// 				slog.String("user_ip", data.UserIP),
// 				slog.String("user_isp", data.UserISP),
// 			)
// 		case err := <-e:
// 			fmt.Println(err)
// 		case <-q:
// 			slog.Info("Received shutdown signal, stopping...")
// 			return
// 		}
// 	}
// }
