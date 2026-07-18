BIN := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X github.com/fox27374/net-lama/internal/version.Version=$(VERSION)"

.PHONY: all build proto vet clean pi

all: build

build:
	go build $(LDFLAGS) -o $(BIN)/netlama-server ./cmd/server
	go build $(LDFLAGS) -o $(BIN)/netlama-agent ./cmd/agent

# Cross-compile the agent for Raspberry Pi
pi:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BIN)/netlama-agent-linux-arm64 ./cmd/agent
	GOOS=linux GOARCH=arm GOARM=7 go build $(LDFLAGS) -o $(BIN)/netlama-agent-linux-armv7 ./cmd/agent

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/netlama.proto

vet:
	go vet ./...

clean:
	rm -rf $(BIN)
