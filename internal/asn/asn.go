// Package asn resolves an IP address to the network that announces it —
// the AS number, its operator's name and the country the AS is registered
// in — from embedded tables, the same shape as internal/oui.
//
// Data source: APNIC's published view of the global routing table
// (https://thyme.apnic.net/current/data-raw-table and
// .../data-used-autnums), fetched 2026-08-05. The plan named iptoasn.com,
// which serves the same data under CC0 but is unreachable from here behind
// a Cloudflare block; APNIC publishes the equivalent snapshot openly.
//
// Regenerating (roughly annually, or when a path shows an unknown network):
//
//	curl -O https://thyme.apnic.net/current/data-raw-table
//	curl -O https://thyme.apnic.net/current/data-used-autnums
//	go run ./internal/asn/gen   # writes asn-ranges.bin.gz + asn-names.tsv.gz
//
// The raw table holds ~1.07M prefixes; contiguous ranges announced by the
// same AS are merged into 370k ranges, which is what makes embedding it
// affordable (2.6MB + 1.1MB gzipped rather than ~35MB of text).
//
// The country is where the *AS is registered*, not where the router sits.
// A Level 3 router in Vienna reports US. Anything better needs a real geo
// database, a licence, and a much larger file — which is why the plan's
// "hop geo" is deliberately only this.
package asn

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	_ "embed"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed asn-ranges.bin.gz
var rangeData []byte

//go:embed asn-names.tsv.gz
var nameData []byte

// Info is what is known about the network announcing an address.
type Info struct {
	ASN     uint32 `json:"asn"`
	Owner   string `json:"owner"`
	Country string `json:"country"`
}

// ipRange is one announced range, as start/end of the IPv4 space.
type ipRange struct {
	start, end uint32
	asn        uint32
}

var (
	once   sync.Once
	ranges []ipRange
	names  map[uint32]Info
)

func load() {
	ranges = make([]ipRange, 0, 380000)
	if zr, err := gzip.NewReader(bytes.NewReader(rangeData)); err == nil {
		blob, err := io.ReadAll(zr)
		if err == nil {
			// Each record is three big-endian uint32s: start, end, asn.
			for i := 0; i+12 <= len(blob); i += 12 {
				ranges = append(ranges, ipRange{
					start: binary.BigEndian.Uint32(blob[i:]),
					end:   binary.BigEndian.Uint32(blob[i+4:]),
					asn:   binary.BigEndian.Uint32(blob[i+8:]),
				})
			}
		}
	}

	names = make(map[uint32]Info, 80000)
	if zr, err := gzip.NewReader(bytes.NewReader(nameData)); err == nil {
		sc := bufio.NewScanner(zr)
		for sc.Scan() {
			parts := strings.Split(sc.Text(), "\t")
			if len(parts) < 2 {
				continue
			}
			n, err := strconv.ParseUint(parts[0], 10, 32)
			if err != nil {
				continue
			}
			info := Info{ASN: uint32(n), Owner: parts[1]}
			if len(parts) > 2 {
				info.Country = parts[2]
			}
			names[uint32(n)] = info
		}
	}
}

// Lookup returns the network announcing ip, or ok=false when the address is
// not in the routing table at all — which is the normal answer for the
// private hops at the start of every trace, not an error.
func Lookup(ip string) (Info, bool) {
	once.Do(load)

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return Info{}, false
	}
	v4 := parsed.To4()
	if v4 == nil {
		return Info{}, false // IPv6 is not in these tables
	}
	addr := binary.BigEndian.Uint32(v4)

	i := sort.Search(len(ranges), func(i int) bool { return ranges[i].end >= addr })
	if i >= len(ranges) || ranges[i].start > addr {
		return Info{}, false
	}
	if info, ok := names[ranges[i].asn]; ok {
		return info, true
	}
	// Announced by an AS with no published name: the number is still useful.
	return Info{ASN: ranges[i].asn}, true
}
