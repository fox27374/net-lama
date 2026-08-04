package testtype

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	pb "github.com/fox27374/net-lama/proto"
)

// TestEveryResultVariantIsRegistered walks the TestResult oneof through
// proto reflection rather than a hand-written list, so adding a result
// variant to netlama.proto without registering its type fails here instead
// of silently storing results as "unknown" (which drops them out of the
// ?type= filter the UI queries by).
func TestEveryResultVariantIsRegistered(t *testing.T) {
	oneof := (&pb.TestResult{}).ProtoReflect().Descriptor().Oneofs().ByName("result")
	if oneof == nil {
		t.Fatal(`TestResult has no "result" oneof — did the proto change?`)
	}

	for i := 0; i < oneof.Fields().Len(); i++ {
		field := oneof.Fields().Get(i)
		t.Run(string(field.Name()), func(t *testing.T) {
			result := &pb.TestResult{}
			result.ProtoReflect().Set(field, protoreflect.ValueOfMessage(
				result.ProtoReflect().NewField(field).Message()))

			typeName := TypeOf(result)
			if typeName == "unknown" {
				t.Fatalf("result variant %q maps to no test type — add a case to resultTypeName and a spec for it", field.Name())
			}
			if Get(typeName) == nil {
				t.Fatalf("result variant %q maps to type %q, which has no registered Spec", field.Name(), typeName)
			}
		})
	}
}

// TestSpecsAreComplete checks the per-type facts callers depend on are
// actually filled in — a spec with no unit renders an unlabelled axis, and
// one whose Primary never yields a value leaves a permanently blank
// sparkline.
func TestSpecsAreComplete(t *testing.T) {
	for _, spec := range All() {
		t.Run(spec.Type, func(t *testing.T) {
			if spec.Capability == "" {
				t.Error("no capability")
			}
			if spec.Unit == "" {
				t.Error("no unit")
			}
			if spec.Primary == nil {
				t.Error("no primary metric")
			}
			// Every metric must tolerate a result of the wrong type rather
			// than panicking: alert rules are evaluated against whatever
			// result arrives, not only matching ones.
			empty := &pb.TestResult{}
			spec.Primary(empty)
			for name, fn := range spec.Metrics {
				if fn == nil {
					t.Errorf("metric %q is nil", name)
					continue
				}
				fn(empty)
			}
			if spec.Subject != nil {
				spec.Subject(empty)
			}
		})
	}
}

// TestPrimaryMetrics pins the number each type reports as "the" value of a
// run. These used to be written out twice — once over the proto in the
// alert engine, once over the JSON payload in the dashboard — and had
// drifted apart for wlan_passive.
func TestPrimaryMetrics(t *testing.T) {
	cases := []struct {
		testType string
		result   *pb.TestResult
		want     float64
		wantOK   bool
	}{
		{"ping", &pb.TestResult{Result: &pb.TestResult_Ping{Ping: &pb.PingResult{AvgRttMs: 12.5}}}, 12.5, true},
		{"dns", &pb.TestResult{Result: &pb.TestResult_Dns{Dns: &pb.DnsResult{ResolveTimeMs: 8}}}, 8, true},
		{"http", &pb.TestResult{Result: &pb.TestResult_Http{Http: &pb.HttpResult{TotalMs: 250}}}, 250, true},
		{"tcp", &pb.TestResult{Result: &pb.TestResult_Tcp{Tcp: &pb.TcpResult{ConnectMs: 3}}}, 3, true},
		{"speedtest", &pb.TestResult{Result: &pb.TestResult_Speedtest{Speedtest: &pb.SpeedtestResult{DownloadMbps: 940}}}, 940, true},
		{"perfmon", &pb.TestResult{Result: &pb.TestResult_Perfmon{Perfmon: &pb.PerfmonResult{DownloadMbps: 720}}}, 720, true},
		{"traceroute", &pb.TestResult{Result: &pb.TestResult_Traceroute{Traceroute: &pb.TracerouteResult{
			Hops: []*pb.Hop{{}, {}, {}},
		}}}, 3, true},
		{"wlan_active", &pb.TestResult{Result: &pb.TestResult_WlanActive{WlanActive: &pb.WlanActiveResult{
			ScanMs: 5000, AssociateMs: 40, AuthenticateMs: 60, DhcpMs: 100,
		}}}, 200, true}, // scan time deliberately excluded
		{"wlan_passive", &pb.TestResult{Result: &pb.TestResult_WlanPassive{WlanPassive: &pb.WlanPassiveResult{
			Channels: []*pb.WlanChannelStat{{UtilizationPct: 12}, {UtilizationPct: 61}, {UtilizationPct: 40}},
			Networks: []*pb.WlanNetwork{{}, {}}, // network count is NOT the metric
		}}}, 61, true},
		// A zero reading means "not measured" for every type, and is
		// reported as absent so it leaves a gap rather than plotting as 0.
		{"ping", &pb.TestResult{Result: &pb.TestResult_Ping{Ping: &pb.PingResult{}}}, 0, false},
	}

	for _, c := range cases {
		t.Run(c.testType, func(t *testing.T) {
			spec := Get(c.testType)
			if spec == nil {
				t.Fatalf("no spec registered for %q", c.testType)
			}
			got, ok := spec.Primary(c.result)
			if got != c.want || ok != c.wantOK {
				t.Errorf("Primary = (%v, %v), want (%v, %v)", got, ok, c.want, c.wantOK)
			}
			// The same result must resolve to the same spec by payload.
			if byResult := OfResult(c.result); byResult != spec {
				t.Errorf("OfResult resolved to %v, want %s", byResult, c.testType)
			}
		})
	}
}

// TestWlanPassivePrimaryIsUtilizationNotNetworkCount is the regression
// guard for the drift this registry exists to prevent: the alert engine
// scored a wlan_passive run by how many networks it heard while the
// dashboard plotted channel utilization, so one "%" threshold meant two
// different things.
func TestWlanPassivePrimaryIsUtilizationNotNetworkCount(t *testing.T) {
	// Seven networks heard, but every channel quiet.
	result := &pb.TestResult{Result: &pb.TestResult_WlanPassive{WlanPassive: &pb.WlanPassiveResult{
		Networks: []*pb.WlanNetwork{{}, {}, {}, {}, {}, {}, {}},
		Channels: []*pb.WlanChannelStat{{UtilizationPct: 3}},
	}}}

	got, ok := Get("wlan_passive").Primary(result)
	if !ok || got != 3 {
		t.Fatalf("Primary = (%v, %v), want (3, true) — utilization, not the 7 networks", got, ok)
	}
	if Get("wlan_passive").Unit != "%" {
		t.Errorf("unit = %q, want %%", Get("wlan_passive").Unit)
	}
}

// TestMetricValueSkipsInapplicableRules covers how an alert rule watching a
// metric its test type doesn't expose gets skipped rather than evaluated
// against a meaningless zero.
func TestMetricValueSkipsInapplicableRules(t *testing.T) {
	ping := &pb.TestResult{Result: &pb.TestResult_Ping{Ping: &pb.PingResult{AvgRttMs: 20, LossPercent: 5}}}

	if v, ok := MetricValue("latency_ms", ping); !ok || v != 20 {
		t.Errorf("latency_ms = (%v, %v), want (20, true)", v, ok)
	}
	if v, ok := MetricValue("loss_percent", ping); !ok || v != 5 {
		t.Errorf("loss_percent = (%v, %v), want (5, true)", v, ok)
	}
	if _, ok := MetricValue("download_mbps", ping); ok {
		t.Error("download_mbps applied to a ping result; the rule should have been skipped")
	}
	if _, ok := MetricValue("latency_ms", &pb.TestResult{}); ok {
		t.Error("a result with no payload yielded a metric")
	}
}

// TestAlertMetricsCoversEveryRegisteredMetric guards the API's rule
// validation, which is derived from this list instead of a second hardcoded
// map.
func TestAlertMetricsCoversEveryRegisteredMetric(t *testing.T) {
	listed := map[string]bool{}
	for _, name := range AlertMetrics() {
		listed[name] = true
	}
	for _, spec := range All() {
		for name := range spec.Metrics {
			if !listed[name] {
				t.Errorf("%s exposes metric %q, which AlertMetrics() omits", spec.Type, name)
			}
		}
	}
}

// TestResultRoundTrip is the property the store depends on: a result
// written to the payload column comes back as the same message, so the
// dashboard reads the same number the alert engine saw.
func TestResultRoundTrip(t *testing.T) {
	original := &pb.TestResult{
		TestId:   "t1",
		TestName: "Ping",
		Result: &pb.TestResult_Ping{Ping: &pb.PingResult{
			Target: "1.1.1.1", PacketsSent: 5, PacketsReceived: 4,
			LossPercent: 20, MinRttMs: 8, AvgRttMs: 12.5, MaxRttMs: 30,
		}},
	}

	payload, err := EncodeResult(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeResult(payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if TypeOf(decoded) != "ping" {
		t.Errorf("decoded type = %q, want ping", TypeOf(decoded))
	}
	want, _ := Get("ping").Primary(original)
	got, ok := Get("ping").Primary(decoded)
	if !ok || got != want {
		t.Errorf("primary metric survived the round trip as (%v, %v), want (%v, true)", got, ok, want)
	}
	if SubjectOf(decoded) != "1.1.1.1" {
		t.Errorf("subject = %q, want 1.1.1.1", SubjectOf(decoded))
	}
}

// TestDecodeToleratesUnknownFields keeps a payload written by a newer
// server loadable by an older one, rather than failing the whole overview.
func TestDecodeToleratesUnknownFields(t *testing.T) {
	if _, err := DecodeResult([]byte(`{"testName":"Ping","somethingNew":1,"ping":{"avgRttMs":4}}`)); err != nil {
		t.Fatalf("decode with an unknown field failed: %v", err)
	}
}

// TestEveryTypeParamsRoundTrip drives every registered type down the path
// ValidateTestDef and TestSpec take: decode the stored JSON, validate it,
// re-encode the normalized form, and turn it into the oneof pushed to an
// agent. A type whose Apply forgets to set the oneof would ship a spec the
// agent's dispatch switch silently drops.
func TestEveryTypeParamsRoundTrip(t *testing.T) {
	// One valid stored payload per type. A new type fails here until it
	// gets an entry, which is the reminder to check its params work.
	valid := map[string]string{
		"ping":         `{"targets":["8.8.8.8"]}`,
		"dns":          `{"queries":["example.com"],"servers":["1.1.1.1"]}`,
		"http":         `{"url":"https://example.com"}`,
		"tcp":          `{"targets":["example.com:443"]}`,
		"speedtest":    `{"provider":"ndt7"}`,
		"perfmon":      `{"sourceAgentId":"a1","target":"10.0.0.2:5201"}`,
		"traceroute":   `{"target":"1.1.1.1"}`,
		"saas":         `{"service":"ms-teams"}`,
		"wlan_passive": `{}`,
		"wlan_active":  `{"ssid":"corp","security":"psk","password":"hunter2"}`,
	}

	for _, spec := range All() {
		t.Run(spec.Type, func(t *testing.T) {
			raw, ok := valid[spec.Type]
			if !ok {
				t.Fatalf("no valid params payload for %q — add one", spec.Type)
			}
			params, err := spec.DecodeParams([]byte(raw))
			if err != nil {
				t.Fatalf("DecodeParams: %v", err)
			}
			if err := params.Validate(); err != nil {
				t.Fatalf("Validate rejected a valid payload: %v", err)
			}

			out := &pb.TestSpec{}
			params.Apply(out)
			if out.Params == nil {
				t.Fatal("Apply set no params oneof — the agent would run nothing")
			}

			// Empty stored params must decode, so a type whose params are
			// all optional needs no JSON at all.
			if _, err := spec.DecodeParams(nil); err != nil {
				t.Fatalf("DecodeParams(nil): %v", err)
			}
		})
	}
}
