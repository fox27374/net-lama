# Multi-stage build for the net-lama server and agent images.
#
#   podman build --target server        -t netlama-server .
#   podman build --target agent         -t netlama-agent .
#   podman build --target agent-sensor  -t netlama-agent-sensor .   # + iw, mtr

FROM docker.io/library/golang:1.25 AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-X github.com/fox27374/net-lama/internal/version.Version=${VERSION}" -o /out/netlama-server ./cmd/server && \
    CGO_ENABLED=0 go build -ldflags "-X github.com/fox27374/net-lama/internal/version.Version=${VERSION}" -o /out/netlama-agent ./cmd/agent

FROM gcr.io/distroless/static-debian12:nonroot AS server
COPY --from=build /out/netlama-server /netlama-server
EXPOSE 50051 9090
VOLUME /data
ENTRYPOINT ["/netlama-server"]
CMD ["-db", "/data/netlama.db"]

FROM gcr.io/distroless/static-debian12:nonroot AS agent
COPY --from=build /out/netlama-agent /netlama-agent
ENTRYPOINT ["/netlama-agent"]

# agent-sensor bundles the external tools the probes shell out to: iw + ip
# (iproute2) for WLAN scan/monitor-mode sensing, mtr for traceroute. Larger
# and not distroless; use this variant on agents that do WLAN sensing or path
# tracing. Needs CAP_NET_RAW (and CAP_NET_ADMIN for WLAN); see the README for
# the host/network requirements.
FROM debian:12-slim AS agent-sensor
RUN apt-get update && \
    apt-get install -y --no-install-recommends iw iproute2 mtr-tiny ca-certificates && \
    rm -rf /var/lib/apt/lists/*
COPY --from=build /out/netlama-agent /netlama-agent
ENTRYPOINT ["/netlama-agent"]
