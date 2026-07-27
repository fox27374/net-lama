package agent

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"strings"
)

// DNS server discovery: a freshly deployed agent finds its server the way a
// Cisco access point finds its controller — the local DNS zone names it, so
// nothing has to be baked into the image except the token. Tried in order:
//
//	SRV  _netlama._tcp   host and port, already sorted by priority/weight
//	A    net-lama        port defaults to discoverPort
//
// Both names are looked up unqualified so the resolver's search domains from
// /etc/resolv.conf apply — "net-lama" means "net-lama in my local domain".
// The A lookup also matches an /etc/hosts entry, which is the easy way to
// test this without touching DNS.
const (
	// DiscoverAddr is the -server value that turns discovery on (the default).
	DiscoverAddr = "auto"

	discoverSRVName  = "_netlama._tcp"
	discoverHostName = "net-lama"
	discoverPort     = 50051
	// discoverFallback keeps a bare `netlama-agent` working on a dev box
	// with nothing in DNS — same address the agent defaulted to before.
	discoverFallback = "localhost:50051"
)

// discoverServer resolves the server address from DNS. It never fails: with
// nothing in DNS it returns discoverFallback, and the caller's reconnect loop
// retries the whole lookup, so an agent booted before its DNS entry exists
// picks the server up on a later attempt.
func discoverServer(ctx context.Context, logger *slog.Logger) string {
	// Empty service and proto make LookupSRV look the name up verbatim,
	// which is what lets the search domains apply.
	if _, recs, err := net.DefaultResolver.LookupSRV(ctx, "", "", discoverSRVName); err == nil {
		if addr := srvAddr(recs); addr != "" {
			logger.Info("Discovered server via DNS SRV",
				slog.String("name", discoverSRVName),
				slog.String("server", addr),
			)
			return addr
		}
	}

	// Only the existence of the record matters — the name itself is handed to
	// gRPC, which resolves it again and picks between multiple A/AAAA records
	// on its own. It also keeps TLS verification against the hostname instead
	// of a bare IP, so the server cert needs no IP SAN.
	if hosts, err := net.DefaultResolver.LookupHost(ctx, discoverHostName); err == nil && len(hosts) > 0 {
		addr := net.JoinHostPort(discoverHostName, strconv.Itoa(discoverPort))
		logger.Info("Discovered server via DNS",
			slog.String("name", discoverHostName),
			slog.Any("addresses", hosts),
			slog.String("server", addr),
		)
		return addr
	}

	logger.Warn("Server not found in DNS, using the fallback address",
		slog.String("srv", discoverSRVName),
		slog.String("host", discoverHostName),
		slog.String("server", discoverFallback),
	)
	return discoverFallback
}

// srvAddr formats the first SRV record as host:port — the resolver returns
// them sorted by priority and weight-shuffled, so records after the first are
// the failover targets and the reconnect loop reaches them by re-resolving.
// Returns "" if there is nothing usable, including the "." target that means
// "this service is explicitly not available here".
func srvAddr(recs []*net.SRV) string {
	if len(recs) == 0 || recs[0] == nil {
		return ""
	}
	host := strings.TrimSuffix(recs[0].Target, ".")
	if host == "" || recs[0].Port == 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(int(recs[0].Port)))
}
