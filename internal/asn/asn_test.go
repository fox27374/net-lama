package asn

import (
	"encoding/binary"
	"net"
	"testing"
)

// TestLookupKnownNetworks pins a few addresses whose announcing AS has been
// stable for years, so a regenerated table that silently loses its ranges
// (a parse change, a truncated download) fails here rather than showing
// every hop as unknown in the UI.
func TestLookupKnownNetworks(t *testing.T) {
	cases := []struct {
		ip      string
		wantASN uint32
		owner   string
	}{
		{"1.1.1.1", 13335, "Cloudflare"},
		{"8.8.8.8", 15169, "Google"},
		{"9.9.9.9", 19281, "Quad9"},
	}
	for _, c := range cases {
		t.Run(c.ip, func(t *testing.T) {
			info, ok := Lookup(c.ip)
			if !ok {
				t.Fatalf("%s is not in the table", c.ip)
			}
			if info.ASN != c.wantASN {
				t.Errorf("ASN = %d, want %d", info.ASN, c.wantASN)
			}
			if info.Owner == "" {
				t.Errorf("no owner name for AS%d", info.ASN)
			}
		})
	}
}

// TestLookupUnannounced covers the normal case at the start of every trace:
// private and unrouted addresses are absent from the routing table, which is
// an answer, not a failure.
func TestLookupUnannounced(t *testing.T) {
	for _, ip := range []string{"10.0.0.1", "192.168.1.1", "127.0.0.1", "not-an-ip", "2001:db8::1"} {
		if info, ok := Lookup(ip); ok {
			t.Errorf("%s resolved to AS%d (%s), want not found", ip, info.ASN, info.Owner)
		}
	}
}

// TestLookupRangeBoundaries checks the binary search at the edges of a
// range rather than only in its middle, where an off-by-one hides.
func TestLookupRangeBoundaries(t *testing.T) {
	once.Do(load)
	if len(ranges) == 0 {
		t.Fatal("no ranges loaded from the embedded table")
	}
	r := ranges[len(ranges)/2]

	for _, tc := range []struct {
		name string
		addr uint32
		want bool
	}{
		{"first address", r.start, true},
		{"last address", r.end, true},
		{"just below", r.start - 1, false},
	} {
		got, ok := Lookup(uint32ToIP(tc.addr))
		if tc.want && (!ok || got.ASN != r.asn) {
			t.Errorf("%s: got (AS%d, %v), want AS%d", tc.name, got.ASN, ok, r.asn)
		}
		if !tc.want && ok && got.ASN == r.asn {
			t.Errorf("%s: address below the range resolved into it", tc.name)
		}
	}
}

func uint32ToIP(v uint32) string {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, v)
	return ip.String()
}
