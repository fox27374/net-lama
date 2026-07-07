package probe

import (
	"context"
	"fmt"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

type PingResult struct {
	Target          string
	PacketsSent     int
	PacketsReceived int
	LossPercent     float64
	MinRttMs        float64
	AvgRttMs        float64
	MaxRttMs        float64
}

// Ping sends count ICMP echo requests to target and returns RTT statistics.
// Runs unprivileged (UDP), so no root is required on the Pi.
func Ping(ctx context.Context, target string, count int) (*PingResult, error) {
	if count <= 0 {
		count = 5
	}

	pinger, err := probing.NewPinger(target)
	if err != nil {
		return nil, fmt.Errorf("creating pinger: %w", err)
	}

	pinger.Count = count
	pinger.Interval = 200 * time.Millisecond
	pinger.Timeout = time.Duration(count)*time.Second + 2*time.Second
	pinger.SetPrivileged(false)

	if err := pinger.RunWithContext(ctx); err != nil {
		return nil, fmt.Errorf("running ping: %w", err)
	}

	stats := pinger.Statistics()
	return &PingResult{
		Target:          target,
		PacketsSent:     stats.PacketsSent,
		PacketsReceived: stats.PacketsRecv,
		LossPercent:     stats.PacketLoss,
		MinRttMs:        float64(stats.MinRtt.Microseconds()) / 1000,
		AvgRttMs:        float64(stats.AvgRtt.Microseconds()) / 1000,
		MaxRttMs:        float64(stats.MaxRtt.Microseconds()) / 1000,
	}, nil
}
