package agent

import (
	"net"
	"testing"
)

func TestSrvAddr(t *testing.T) {
	tests := []struct {
		name string
		recs []*net.SRV
		want string
	}{
		{"none", nil, ""},
		{"trailing dot stripped", []*net.SRV{{Target: "netlama.corp.local.", Port: 50051}}, "netlama.corp.local:50051"},
		{"first record wins", []*net.SRV{
			{Target: "a.corp.local.", Port: 50051},
			{Target: "b.corp.local.", Port: 50052},
		}, "a.corp.local:50051"},
		{"root target means unavailable", []*net.SRV{{Target: ".", Port: 50051}}, ""},
		{"zero port", []*net.SRV{{Target: "netlama.corp.local.", Port: 0}}, ""},
	}
	for _, tt := range tests {
		if got := srvAddr(tt.recs); got != tt.want {
			t.Errorf("%s: srvAddr() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
