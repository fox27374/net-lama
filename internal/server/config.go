package server

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/fox27374/net-lama/internal/store"
	pb "github.com/fox27374/net-lama/proto"
)

// Test parameter payloads as stored in the database (tests.params).
type PingParams struct {
	Targets []string `json:"targets"`
	Count   uint32   `json:"count"`
}

type DNSParams struct {
	Queries []string `json:"queries"`
	Servers []string `json:"servers"`
}

type HTTPParams struct {
	URL            string `json:"url"`
	TimeoutSeconds uint32 `json:"timeoutSeconds"`
	SkipTLSVerify  bool   `json:"skipTlsVerify"`
}

type TCPParams struct {
	Targets        []string `json:"targets"`
	TimeoutSeconds uint32   `json:"timeoutSeconds"`
}

// ValidateTestDef checks type, interval and the type-specific parameters
// and returns the definition with normalized params.
func ValidateTestDef(t *store.TestDef) error {
	if t.Name == "" {
		return fmt.Errorf("test name is required")
	}
	if t.IntervalSeconds < 5 {
		return fmt.Errorf("interval must be at least 5 seconds")
	}

	switch t.Type {
	case "speedtest":
		if t.IntervalSeconds < 60 {
			return fmt.Errorf("speedtest interval must be at least 60 seconds")
		}
		t.Params = json.RawMessage(`{}`)

	case "ping":
		var p PingParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return fmt.Errorf("invalid ping parameters: %w", err)
		}
		if len(p.Targets) == 0 {
			return fmt.Errorf("ping requires at least one target")
		}
		if p.Count == 0 {
			p.Count = 5
		}
		if p.Count > 20 {
			return fmt.Errorf("ping count must be at most 20")
		}
		normalized, _ := json.Marshal(p)
		t.Params = normalized

	case "dns":
		var p DNSParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return fmt.Errorf("invalid dns parameters: %w", err)
		}
		if len(p.Queries) == 0 || len(p.Servers) == 0 {
			return fmt.Errorf("dns requires at least one query and one server")
		}
		normalized, _ := json.Marshal(p)
		t.Params = normalized

	case "http":
		var p HTTPParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return fmt.Errorf("invalid http parameters: %w", err)
		}
		p.URL = strings.TrimSpace(p.URL)
		if !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
			return fmt.Errorf("http url must start with http:// or https://")
		}
		if p.TimeoutSeconds == 0 {
			p.TimeoutSeconds = 10
		}
		normalized, _ := json.Marshal(p)
		t.Params = normalized

	case "tcp":
		var p TCPParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return fmt.Errorf("invalid tcp parameters: %w", err)
		}
		if len(p.Targets) == 0 {
			return fmt.Errorf("tcp requires at least one target")
		}
		for _, target := range p.Targets {
			if _, _, err := net.SplitHostPort(target); err != nil {
				return fmt.Errorf("tcp target %q must be host:port", target)
			}
		}
		if p.TimeoutSeconds == 0 {
			p.TimeoutSeconds = 5
		}
		normalized, _ := json.Marshal(p)
		t.Params = normalized

	case "wlan_scan":
		if t.IntervalSeconds < 30 {
			return fmt.Errorf("wlan scan interval must be at least 30 seconds")
		}
		t.Params = json.RawMessage(`{}`)

	default:
		return fmt.Errorf("unknown test type %q", t.Type)
	}
	return nil
}

// TestSpec converts a stored test definition to the protobuf spec
// pushed down the control stream.
func TestSpec(t *store.TestDef) (*pb.TestSpec, error) {
	spec := &pb.TestSpec{
		Id:              t.ID,
		Name:            t.Name,
		IntervalSeconds: t.IntervalSeconds,
	}

	switch t.Type {
	case "speedtest":
		spec.Params = &pb.TestSpec_Speedtest{Speedtest: &pb.SpeedtestParams{}}
	case "ping":
		var p PingParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return nil, err
		}
		spec.Params = &pb.TestSpec_Ping{Ping: &pb.PingParams{Targets: p.Targets, Count: p.Count}}
	case "dns":
		var p DNSParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return nil, err
		}
		spec.Params = &pb.TestSpec_Dns{Dns: &pb.DnsParams{Queries: p.Queries, Servers: p.Servers}}
	case "http":
		var p HTTPParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return nil, err
		}
		spec.Params = &pb.TestSpec_Http{Http: &pb.HttpParams{
			Url: p.URL, TimeoutSeconds: p.TimeoutSeconds, SkipTlsVerify: p.SkipTLSVerify,
		}}
	case "tcp":
		var p TCPParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return nil, err
		}
		spec.Params = &pb.TestSpec_Tcp{Tcp: &pb.TcpParams{
			Targets: p.Targets, TimeoutSeconds: p.TimeoutSeconds,
		}}
	case "wlan_scan":
		spec.Params = &pb.TestSpec_WlanScan{WlanScan: &pb.WlanScanParams{}}
	default:
		return nil, fmt.Errorf("unknown test type %q", t.Type)
	}
	return spec, nil
}
