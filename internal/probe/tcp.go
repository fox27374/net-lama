package probe

import (
	"context"
	"net"
	"time"
)

type TCPResult struct {
	Target    string
	Connected bool
	ConnectMs float64
}

// TCPConnect measures how long a TCP connection to target (host:port)
// takes. A refused/failed connection is returned as Connected=false
// with the probe error, so it can still be recorded as a measurement.
func TCPConnect(ctx context.Context, target string, timeoutSeconds uint32) (*TCPResult, error) {
	if timeoutSeconds == 0 {
		timeoutSeconds = 5
	}
	dialer := net.Dialer{Timeout: time.Duration(timeoutSeconds) * time.Second}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", target)
	elapsed := float64(time.Since(start).Microseconds()) / 1000
	if err != nil {
		return &TCPResult{Target: target, Connected: false, ConnectMs: elapsed}, err
	}
	conn.Close()

	return &TCPResult{Target: target, Connected: true, ConnectMs: elapsed}, nil
}
