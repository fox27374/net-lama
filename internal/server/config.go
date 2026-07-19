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

type SpeedtestParams struct {
	// Provider selects the speedtest backend. Empty means "ookla" (the
	// existing default), keeping every pre-existing speedtest test
	// working unchanged.
	Provider string `json:"provider"`
}

type WlanPassiveParams struct {
}

type WlanActiveParams struct {
	SSID               string `json:"ssid"`
	Security           string `json:"security"` // "psk", "eap-peap", "open"
	Password           string `json:"password"`
	Identity           string `json:"identity"`
	CACertPEM          string `json:"caCertPem"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	ThroughputURL      string `json:"throughputUrl"`
	MACMode            string `json:"macMode"` // "permanent" (default) or "random"
}

type TracerouteParams struct {
	Target       string `json:"target"`
	Protocol     string `json:"protocol"`
	Port         uint32 `json:"port"`
	MaxHops      uint32 `json:"maxHops"`
	ProbesPerHop uint32 `json:"probesPerHop"`
}

// Thresholds represents warn/crit boundaries for a test.
type Thresholds struct {
	Warn float64 `json:"warn"`
	Crit float64 `json:"crit"`
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

	// Validate thresholds if present
	if len(t.Thresholds) > 0 {
		var th Thresholds
		if err := json.Unmarshal(t.Thresholds, &th); err != nil {
			return fmt.Errorf("invalid thresholds: %w", err)
		}
		if th.Warn > 0 && th.Crit > 0 {
			// speedtest is lower-is-worse: orange below warn, red below crit
			if t.Type == "speedtest" {
				if th.Warn <= th.Crit {
					return fmt.Errorf("warn threshold must be greater than crit threshold for speedtest")
				}
			} else if th.Warn >= th.Crit {
				return fmt.Errorf("warn threshold must be less than crit threshold")
			}
		}
	}

	switch t.Type {
	case "speedtest":
		if t.IntervalSeconds < 60 {
			return fmt.Errorf("speedtest interval must be at least 60 seconds")
		}
		var p SpeedtestParams
		if len(t.Params) > 0 {
			if err := json.Unmarshal(t.Params, &p); err != nil {
				return fmt.Errorf("invalid speedtest parameters: %w", err)
			}
		}
		switch p.Provider {
		case "", "ookla", "ndt7", "cloudflare":
		default:
			return fmt.Errorf("speedtest provider must be ookla, ndt7 or cloudflare")
		}
		normalized, _ := json.Marshal(p)
		t.Params = normalized

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

	case "wlan_passive":
		if t.IntervalSeconds < 60 {
			return fmt.Errorf("wlan_passive interval must be at least 60 seconds")
		}
		t.Params = json.RawMessage(`{}`)

	case "wlan_active":
		var p WlanActiveParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return fmt.Errorf("invalid wlan_active parameters: %w", err)
		}
		p.SSID = strings.TrimSpace(p.SSID)
		if p.SSID == "" {
			return fmt.Errorf("wlan_active requires an SSID")
		}
		switch p.Security {
		case "":
			p.Security = "psk"
		case "psk", "eap-peap", "open":
		default:
			return fmt.Errorf("wlan_active security must be psk, eap-peap or open")
		}
		if p.Security == "psk" && p.Password == "" {
			return fmt.Errorf("wlan_active with psk requires a password")
		}
		if p.Security == "eap-peap" {
			if p.Identity == "" || p.Password == "" {
				return fmt.Errorf("wlan_active with eap-peap requires identity and password")
			}
			if p.CACertPEM == "" && !p.InsecureSkipVerify {
				return fmt.Errorf("wlan_active with eap-peap requires a CA certificate or insecureSkipVerify")
			}
		}
		switch p.MACMode {
		case "":
			p.MACMode = "permanent"
		case "permanent", "random":
		default:
			return fmt.Errorf("wlan_active macMode must be permanent or random")
		}
		// The test takes the radio away from passive sweeps; keep it rare.
		if t.IntervalSeconds < 300 {
			return fmt.Errorf("wlan_active interval must be at least 300 seconds")
		}
		normalized, _ := json.Marshal(p)
		t.Params = normalized

	case "traceroute":
		var p TracerouteParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return fmt.Errorf("invalid traceroute parameters: %w", err)
		}
		p.Target = strings.TrimSpace(p.Target)
		if p.Target == "" {
			return fmt.Errorf("traceroute requires a target")
		}
		switch p.Protocol {
		case "":
			p.Protocol = "tcp"
		case "icmp", "tcp", "udp":
		default:
			return fmt.Errorf("traceroute protocol must be icmp, tcp or udp")
		}
		if (p.Protocol == "tcp" || p.Protocol == "udp") && p.Port == 0 {
			p.Port = 443
		}
		if p.MaxHops == 0 {
			p.MaxHops = 30
		}
		if p.MaxHops > 64 {
			return fmt.Errorf("maxHops must be at most 64")
		}
		if p.ProbesPerHop == 0 {
			p.ProbesPerHop = 5
		}
		if t.IntervalSeconds < 30 {
			return fmt.Errorf("traceroute interval must be at least 30 seconds")
		}
		normalized, _ := json.Marshal(p)
		t.Params = normalized

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
		var p SpeedtestParams
		if len(t.Params) > 0 {
			if err := json.Unmarshal(t.Params, &p); err != nil {
				return nil, err
			}
		}
		spec.Params = &pb.TestSpec_Speedtest{Speedtest: &pb.SpeedtestParams{Provider: p.Provider}}
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
	case "wlan_passive":
		spec.Params = &pb.TestSpec_WlanPassive{WlanPassive: &pb.WlanPassiveParams{}}
	case "wlan_active":
		var p WlanActiveParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return nil, err
		}
		spec.Params = &pb.TestSpec_WlanActive{WlanActive: &pb.WlanActiveParams{
			Ssid: p.SSID, Security: p.Security, Password: p.Password,
			Identity: p.Identity, CaCertPem: p.CACertPEM,
			InsecureSkipVerify: p.InsecureSkipVerify, ThroughputUrl: p.ThroughputURL,
			MacMode: p.MACMode,
		}}
	case "traceroute":
		var p TracerouteParams
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return nil, err
		}
		spec.Params = &pb.TestSpec_Traceroute{Traceroute: &pb.TracerouteParams{
			Target: p.Target, Protocol: p.Protocol, Port: p.Port,
			MaxHops: p.MaxHops, ProbesPerHop: p.ProbesPerHop,
		}}
	default:
		return nil, fmt.Errorf("unknown test type %q", t.Type)
	}
	return spec, nil
}
