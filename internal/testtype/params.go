package testtype

import (
	"fmt"
	"net"
	"strings"

	pb "github.com/fox27374/net-lama/proto"
)

// The stored parameter payloads (the JSON in tests.params). Each one
// validates itself and knows how to become the proto oneof pushed to an
// agent, so neither job needs a switch on the type name.

type PingParams struct {
	Targets []string `json:"targets"`
	Count   uint32   `json:"count"`
}

func (p *PingParams) Validate() error {
	if len(p.Targets) == 0 {
		return fmt.Errorf("ping requires at least one target")
	}
	if p.Count == 0 {
		p.Count = 5
	}
	if p.Count > 20 {
		return fmt.Errorf("ping count must be at most 20")
	}
	return nil
}

func (p *PingParams) Apply(spec *pb.TestSpec) {
	spec.Params = &pb.TestSpec_Ping{Ping: &pb.PingParams{
		Targets: p.Targets, Count: p.Count,
	}}
}

type DNSParams struct {
	Queries []string `json:"queries"`
	Servers []string `json:"servers"`
}

func (p *DNSParams) Validate() error {
	if len(p.Queries) == 0 || len(p.Servers) == 0 {
		return fmt.Errorf("dns requires at least one query and one server")
	}
	return nil
}

func (p *DNSParams) Apply(spec *pb.TestSpec) {
	spec.Params = &pb.TestSpec_Dns{Dns: &pb.DnsParams{
		Queries: p.Queries, Servers: p.Servers,
	}}
}

type HTTPParams struct {
	URL            string `json:"url"`
	TimeoutSeconds uint32 `json:"timeoutSeconds"`
	SkipTLSVerify  bool   `json:"skipTlsVerify"`
}

func (p *HTTPParams) Validate() error {
	p.URL = strings.TrimSpace(p.URL)
	if !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
		return fmt.Errorf("http url must start with http:// or https://")
	}
	if p.TimeoutSeconds == 0 {
		p.TimeoutSeconds = 10
	}
	return nil
}

func (p *HTTPParams) Apply(spec *pb.TestSpec) {
	spec.Params = &pb.TestSpec_Http{Http: &pb.HttpParams{
		Url: p.URL, TimeoutSeconds: p.TimeoutSeconds, SkipTlsVerify: p.SkipTLSVerify,
	}}
}

type TCPParams struct {
	Targets        []string `json:"targets"`
	TimeoutSeconds uint32   `json:"timeoutSeconds"`
}

func (p *TCPParams) Validate() error {
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
	return nil
}

func (p *TCPParams) Apply(spec *pb.TestSpec) {
	spec.Params = &pb.TestSpec_Tcp{Tcp: &pb.TcpParams{
		Targets: p.Targets, TimeoutSeconds: p.TimeoutSeconds,
	}}
}

type SpeedtestParams struct {
	// Provider selects the speedtest backend. Empty means "ookla" (the
	// existing default), keeping every pre-existing speedtest test
	// working unchanged.
	Provider string `json:"provider"`
}

func (p *SpeedtestParams) Validate() error {
	switch p.Provider {
	case "", "ookla", "ndt7", "cloudflare":
		return nil
	}
	return fmt.Errorf("speedtest provider must be ookla, ndt7 or cloudflare")
}

func (p *SpeedtestParams) Apply(spec *pb.TestSpec) {
	spec.Params = &pb.TestSpec_Speedtest{Speedtest: &pb.SpeedtestParams{Provider: p.Provider}}
}

type WlanPassiveParams struct{}

func (p *WlanPassiveParams) Validate() error { return nil }

func (p *WlanPassiveParams) Apply(spec *pb.TestSpec) {
	spec.Params = &pb.TestSpec_WlanPassive{WlanPassive: &pb.WlanPassiveParams{}}
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

func (p *WlanActiveParams) Validate() error {
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
	return nil
}

func (p *WlanActiveParams) Apply(spec *pb.TestSpec) {
	spec.Params = &pb.TestSpec_WlanActive{WlanActive: &pb.WlanActiveParams{
		Ssid: p.SSID, Security: p.Security, Password: p.Password,
		Identity: p.Identity, CaCertPem: p.CACertPEM,
		InsecureSkipVerify: p.InsecureSkipVerify, ThroughputUrl: p.ThroughputURL,
		MacMode: p.MACMode,
	}}
}

type PerfmonParams struct {
	// SourceAgentID pins this test to run on exactly one agent (unlike
	// every other test type, which runs on all agents of an assigned
	// site) — measuring throughput FROM a specific agent TO Target only
	// makes sense as a single-agent test. Existence/tenant-ownership is
	// checked at the API layer (internal/api/sites.go), which has store
	// access; this package only checks the field is non-empty.
	SourceAgentID   string `json:"sourceAgentId"`
	Target          string `json:"target"` // another agent's perfmon reflector, host:port
	DurationSeconds uint32 `json:"durationSeconds"`
}

func (p *PerfmonParams) Validate() error {
	p.SourceAgentID = strings.TrimSpace(p.SourceAgentID)
	if p.SourceAgentID == "" {
		return fmt.Errorf("perfmon requires a source agent")
	}
	p.Target = strings.TrimSpace(p.Target)
	if p.Target == "" {
		return fmt.Errorf("perfmon requires a target (another agent's host:port)")
	}
	if _, _, err := net.SplitHostPort(p.Target); err != nil {
		return fmt.Errorf("perfmon target %q must be host:port", p.Target)
	}
	if p.DurationSeconds == 0 {
		p.DurationSeconds = 5
	}
	if p.DurationSeconds > 30 {
		return fmt.Errorf("perfmon durationSeconds must be at most 30")
	}
	return nil
}

// Apply drops SourceAgentID: the agent that receives the spec is the
// source by construction (ConfigForAgent only pushes it there).
func (p *PerfmonParams) Apply(spec *pb.TestSpec) {
	spec.Params = &pb.TestSpec_Perfmon{Perfmon: &pb.PerfmonParams{
		Target: p.Target, DurationSeconds: p.DurationSeconds,
	}}
}

type TracerouteParams struct {
	Target       string `json:"target"`
	Protocol     string `json:"protocol"`
	Port         uint32 `json:"port"`
	MaxHops      uint32 `json:"maxHops"`
	ProbesPerHop uint32 `json:"probesPerHop"`
}

func (p *TracerouteParams) Validate() error {
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
	return nil
}

func (p *TracerouteParams) Apply(spec *pb.TestSpec) {
	spec.Params = &pb.TestSpec_Traceroute{Traceroute: &pb.TracerouteParams{
		Target: p.Target, Protocol: p.Protocol, Port: p.Port,
		MaxHops: p.MaxHops, ProbesPerHop: p.ProbesPerHop,
	}}
}
