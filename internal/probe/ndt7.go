package probe

import (
	"context"
	"fmt"

	ndt7 "github.com/m-lab/ndt7-client-go"
	"github.com/m-lab/ndt7-client-go/spec"
)

// clientName/clientVersion identify this agent to M-Lab's ndt7 servers, as
// required by the ndt7 spec.
const (
	ndt7ClientName    = "net-lama"
	ndt7ClientVersion = "0.1.0"
)

// NDT7 runs a full ndt7 measurement (download then upload) against the
// nearest M-Lab server, discovered via the public Locate API.
func NDT7(ctx context.Context) (*SpeedtestResult, error) {
	client := ndt7.NewClient(ndt7ClientName, ndt7ClientVersion)

	downloadMbps, err := ndt7RunTest(ctx, client.StartDownload)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	uploadMbps, err := ndt7RunTest(ctx, client.StartUpload)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}

	if downloadMbps <= 0 || uploadMbps <= 0 {
		return nil, fmt.Errorf("returned an implausible measurement (download %.2f Mbps, upload %.2f Mbps)", downloadMbps, uploadMbps)
	}

	return &SpeedtestResult{
		ServerName:   client.FQDN,
		LatencyMs:    ndt7MinRTT(client),
		DownloadMbps: downloadMbps,
		UploadMbps:   uploadMbps,
	}, nil
}

// ndt7RunTest drains a measurement channel started by start (either
// StartDownload or StartUpload) and returns the throughput in Mbps computed
// from the last client-side AppInfo measurement (bytes transferred over
// elapsed time), which is how the ndt7 spec recommends reporting goodput.
func ndt7RunTest(ctx context.Context, start func(context.Context) (<-chan spec.Measurement, error)) (float64, error) {
	ch, err := start(ctx)
	if err != nil {
		return 0, err
	}

	var last spec.Measurement
	for m := range ch {
		if m.Origin == spec.OriginClient && m.AppInfo != nil {
			last = m
		}
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if last.AppInfo == nil || last.AppInfo.ElapsedTime <= 0 {
		return 0, fmt.Errorf("no client-side application-level measurement received")
	}

	// AppInfo.NumBytes is total bytes, ElapsedTime is microseconds.
	bits := float64(last.AppInfo.NumBytes) * 8
	seconds := float64(last.AppInfo.ElapsedTime) / 1e6
	if seconds <= 0 {
		return 0, fmt.Errorf("invalid elapsed time in measurement")
	}
	return bits / seconds / 1e6, nil
}

// ndt7MinRTT extracts the minimum RTT observed during the download test from
// the server-side TCPInfo measurement, if the server sent one. Returns 0 if
// unavailable.
func ndt7MinRTT(client *ndt7.Client) float64 {
	results := client.Results()
	dl, ok := results[spec.TestDownload]
	if !ok || dl.Server.TCPInfo == nil {
		return 0
	}
	// RTT is reported in microseconds by TCP_INFO.
	return float64(dl.Server.TCPInfo.RTT) / 1000
}
