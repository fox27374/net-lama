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
// time the lookup takes. A failed lookup is returned as a result with
// Success=false, not as an error, so it can be reported as a measurement.
func DNSQuery(ctx context.Context, query, server string) *DNSResult {
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

	result := &DNSResult{
		Query:         query,
		Server:        server,
		ResolveTimeMs: float64(elapsed.Microseconds()) / 1000,
	}
	if err == nil {
		result.Success = true
		result.Addresses = addrs
	}
	return result
}
