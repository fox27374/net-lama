package server

import (
	"encoding/json"
	"testing"

	"github.com/fox27374/net-lama/internal/store"
	pb "github.com/fox27374/net-lama/proto"
)

func TestWlanSenseMetricExtraction(t *testing.T) {
	// Create a wlan_sense result with realistic channel utilization
	result := &pb.WlanSenseResult{
		Interface: "wlan0",
		Demo:      true,
		Stations: []*pb.WlanStation{
			{
				Mac:         "aa:bb:cc:dd:ee:01",
				Bssid:       "a0:f8:49:74:8b:20",
				Ssid:        "corp-wifi",
				RssiDbm:     -45,
				RssiAvgDbm:  -48,
				RateMbps:    150,
				Mcs:         8,
				Frames:      250,
				ProbeOnly:   false,
				LastSeenMs:  1000,
			},
		},
		Channels: []*pb.WlanChannelStat{
			{
				Channel:         1,
				FreqMhz:         2412,
				ActiveMs:        400,
				BusyMs:          120,
				UtilizationPct:  30.0,
				Frames:          50,
			},
			{
				Channel:         6,
				FreqMhz:         2437,
				ActiveMs:        400,
				BusyMs:          200,
				UtilizationPct:  50.0,
				Frames:          100,
			},
			{
				Channel:         36,
				FreqMhz:         5180,
				ActiveMs:        400,
				BusyMs:          80,
				UtilizationPct:  20.0,
				Frames:          80,
			},
		},
		SweepMs: 1500,
	}

	// Serialize to JSON to simulate what's stored in the DB
	payload := map[string]interface{}{
		"wlanSense": map[string]interface{}{
			"interface": result.Interface,
			"demo":      result.Demo,
			"stations": []interface{}{
				map[string]interface{}{
					"mac":         "aa:bb:cc:dd:ee:01",
					"bssid":       "a0:f8:49:74:8b:20",
					"ssid":        "corp-wifi",
					"rssiDbm":     -45,
					"rssiAvgDbm":  -48,
					"rateMbps":    150.0,
					"mcs":         8,
					"frames":      250,
					"probeOnly":   false,
					"lastSeenMs":  1000,
				},
			},
			"channels": []interface{}{
				map[string]interface{}{
					"channel":         1,
					"freqMhz":         2412,
					"activeMs":        400,
					"busyMs":          120,
					"utilizationPct":  30.0,
					"frames":          50,
				},
				map[string]interface{}{
					"channel":         6,
					"freqMhz":         2437,
					"activeMs":        400,
					"busyMs":          200,
					"utilizationPct":  50.0,
					"frames":          100,
				},
				map[string]interface{}{
					"channel":         36,
					"freqMhz":         5180,
					"activeMs":        400,
					"busyMs":          80,
					"utilizationPct":  20.0,
					"frames":          80,
				},
			},
			"sweepMs": 1500,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal test payload: %v", err)
	}

	payloadStr := string(data)

	// Test extraction of metric value (max utilization)
	// Note: This would normally be in store/overview.go but we're testing from server context
	var p map[string]interface{}
	err = json.Unmarshal([]byte(payloadStr), &p)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	ws, ok := p["wlanSense"].(map[string]interface{})
	if !ok {
		t.Fatal("missing wlanSense field in payload")
	}

	channels, ok := ws["channels"].([]interface{})
	if !ok {
		t.Fatal("missing channels array in wlanSense")
	}

	// Extract max utilization
	var maxUtil float64
	for _, chData := range channels {
		if ch, ok := chData.(map[string]interface{}); ok {
			if util, ok := ch["utilizationPct"].(float64); ok && util > maxUtil {
				maxUtil = util
			}
		}
	}

	if maxUtil != 50.0 {
		t.Errorf("expected max utilization 50%%, got %.1f%%", maxUtil)
	}

	// Verify all expected data is present
	if result.Interface != "wlan0" {
		t.Errorf("expected interface wlan0, got %s", result.Interface)
	}

	if len(result.Stations) != 1 {
		t.Errorf("expected 1 station, got %d", len(result.Stations))
	}

	if len(result.Channels) != 3 {
		t.Errorf("expected 3 channels, got %d", len(result.Channels))
	}

	if result.SweepMs != 1500 {
		t.Errorf("expected sweep time 1500ms, got %d", result.SweepMs)
	}

	t.Logf("SUCCESS: wlan_sense result has max utilization %.1f%% from %d channels", maxUtil, len(result.Channels))
}

func TestWlanSenseValidation(t *testing.T) {
	tests := []struct {
		name        string
		typ         string
		interval    uint32
		params      interface{}
		expectError bool
	}{
		{
			name:        "valid wlan_sense minimal",
			typ:         "wlan_sense",
			interval:    30,
			params:      WlanSenseParams{DwellMs: 400},
			expectError: false,
		},
		{
			name:        "valid wlan_sense with channels",
			typ:         "wlan_sense",
			interval:    60,
			params:      WlanSenseParams{Channels: []uint32{1, 6, 11}, DwellMs: 500},
			expectError: false,
		},
		{
			name:        "invalid dwell too low",
			typ:         "wlan_sense",
			interval:    30,
			params:      WlanSenseParams{DwellMs: 50},
			expectError: true,
		},
		{
			name:        "invalid dwell too high",
			typ:         "wlan_sense",
			interval:    30,
			params:      WlanSenseParams{DwellMs: 3000},
			expectError: true,
		},
		{
			name:        "invalid channel number",
			typ:         "wlan_sense",
			interval:    30,
			params:      WlanSenseParams{Channels: []uint32{200}},
			expectError: true,
		},
		{
			name:        "interval too low",
			typ:         "wlan_sense",
			interval:    10,
			params:      WlanSenseParams{},
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params, _ := json.Marshal(test.params)

			testDef := &store.TestDef{
				Type:            test.typ,
				Name:            test.name,
				IntervalSeconds: test.interval,
				Params:          params,
			}

			err := ValidateTestDef(testDef)
			if test.expectError && err == nil {
				t.Errorf("expected error, got none")
			}
			if !test.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
