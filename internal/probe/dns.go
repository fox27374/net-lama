package probe

import (
	"context"
	"net"
	"time"
)

type DNSResult struct {
	Query         string
	Server        string
	Success       bool
	ResolveTimeMs float64
	Addresses     []string
}

// DNSQuery resolves query against a specific DNS server and measures the
// time the lookup takes.
//
// A failed lookup is a result with Success=false, not an error — that is
// the measurement. The error return is for the run being abandoned
// (ctx cancelled: the agent is shutting down or its config changed), where
// there is no measurement to report at all.
func DNSQuery(ctx context.Context, query, server string) (*DNSResult, error) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, network, net.JoinHostPort(server, "53"))
		},
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()
	addrs, err := resolver.LookupHost(lookupCtx, query)
	elapsed := time.Since(start)

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	result := &DNSResult{
		Query:         query,
		Server:        server,
		ResolveTimeMs: float64(elapsed.Microseconds()) / 1000,
	}
	if err == nil {
		result.Success = true
		result.Addresses = addrs
	}
	return result, nil
}
