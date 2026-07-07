# Multi-stage build for the net-lama server and agent images.
#
#   podman build --target server -t netlama-server .
#   podman build --target agent  -t netlama-agent .

FROM docker.io/library/golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/netlama-server ./cmd/server && \
    CGO_ENABLED=0 go build -o /out/netlama-agent ./cmd/agent

FROM gcr.io/distroless/static-debian12:nonroot AS server
COPY --from=build /out/netlama-server /netlama-server
EXPOSE 50051 9090
VOLUME /data
ENTRYPOINT ["/netlama-server"]
CMD ["-db", "/data/netlama.db"]

FROM gcr.io/distroless/static-debian12:nonroot AS agent
COPY --from=build /out/netlama-agent /netlama-agent
ENTRYPOINT ["/netlama-agent"]
