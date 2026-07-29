BIN := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X github.com/fox27374/net-lama/internal/version.Version=$(VERSION)"

NFPM ?= go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.47.0
# Package versions must not start with "v" and must look like a version;
# untagged builds (git describe -> "f2eb92a") become 0.0.0-<describe>.
PKG_VERSION := $(shell echo "$(VERSION)" | sed 's/^v//;s/^[^0-9]/0.0.0-&/')

.PHONY: all build proto vet clean pi pkg

all: build

build:
	go build $(LDFLAGS) -o $(BIN)/netlama-server ./cmd/server
	go build $(LDFLAGS) -o $(BIN)/netlama-agent ./cmd/agent

# Cross-compile the agent for Raspberry Pi
pi:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BIN)/netlama-agent-linux-arm64 ./cmd/agent
	GOOS=linux GOARCH=arm GOARM=7 go build $(LDFLAGS) -o $(BIN)/netlama-agent-linux-armv7 ./cmd/agent

# Native agent packages for amd64, arm64 and armv7 (deb + rpm) in dist/
pkg: pi
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BIN)/netlama-agent-linux-amd64 ./cmd/agent
	mkdir -p dist/build
	@set -e; for pair in amd64:amd64 arm64:arm64 armv7:arm7; do \
		cp $(BIN)/netlama-agent-linux-$${pair%%:*} dist/build/netlama-agent; \
		for fmt in deb rpm; do \
			echo "packaging $${pair##*:} $$fmt"; \
			NETLAMA_PKG_ARCH=$${pair##*:} NETLAMA_PKG_VERSION=$(PKG_VERSION) \
				$(NFPM) pkg -f packaging/nfpm.yaml -p $$fmt -t dist/; \
		done; \
	done
	rm -rf dist/build

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/netlama.proto

vet:
	go vet ./...

clean:
	rm -rf $(BIN)
