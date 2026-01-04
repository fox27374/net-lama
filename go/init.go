package main

import (
	"log/slog"
	"os"
)

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
