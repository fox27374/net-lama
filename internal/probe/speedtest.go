package probe

import (
	"context"
	"fmt"

	"github.com/showwin/speedtest-go/speedtest"
)

// maxServerAttempts limits how many candidate servers are tried before
// the speedtest is reported as failed.
const maxServerAttempts = 3

type SpeedtestResult struct {
	ServerName    string
	ServerCountry string
	LatencyMs     float64
	DownloadMbps  float64
	UploadMbps    float64
	UserIP        string
	UserISP       string
}

// Speedtest runs a full speedtest (ping, download, upload) against the
// closest speedtest.net server.
func Speedtest(ctx context.Context) (*SpeedtestResult, error) {
	client := speedtest.New()

	user, err := client.FetchUserInfoContext(ctx)
	if err != nil {
		// User info is informational only, continue without it
		user = &speedtest.User{}
	}

	serverList, err := client.FetchServerListContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching server list: %w", err)
	}

	if len(serverList) == 0 {
		return nil, fmt.Errorf("no speedtest server found")
	}

	// Some test servers accept the connection but deliver a zero result
	// without an error, so try the closest servers until one measures.
	// Note: FindServer would return a single server only; iterate the
	// distance-sorted list instead to have real fallback candidates.
	targets := serverList
	if len(targets) > maxServerAttempts {
		targets = targets[:maxServerAttempts]
	}

	var lastErr error
	for _, s := range targets {
		result, err := runServerTest(ctx, s)
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			lastErr = fmt.Errorf("server %q: %w", s.Name, err)
			continue
		}
		result.UserIP = user.IP
		result.UserISP = user.Isp
		return result, nil
	}
	return nil, lastErr
}

func runServerTest(ctx context.Context, s *speedtest.Server) (*SpeedtestResult, error) {
	if err := s.PingTestContext(ctx, nil); err != nil {
		return nil, fmt.Errorf("ping test: %w", err)
	}
	if err := s.DownloadTestContext(ctx); err != nil {
		return nil, fmt.Errorf("download test: %w", err)
	}
	if err := s.UploadTestContext(ctx); err != nil {
		return nil, fmt.Errorf("upload test: %w", err)
	}
	// Broken test servers deliver zero or near-zero readings for one
	// direction while the other measures fine — treat both as invalid.
	dl, ul := s.DLSpeed.Mbps(), s.ULSpeed.Mbps()
	if dl == 0 || ul == 0 || (dl < 0.1 && ul > 1) || (ul < 0.1 && dl > 1) {
		return nil, fmt.Errorf("returned an implausible measurement (download %.2f Mbps, upload %.2f Mbps)", dl, ul)
	}

	return &SpeedtestResult{
		ServerName:    s.Name,
		ServerCountry: s.Country,
		LatencyMs:     float64(s.Latency.Milliseconds()),
		DownloadMbps:  s.DLSpeed.Mbps(),
		UploadMbps:    s.ULSpeed.Mbps(),
	}, nil
}
