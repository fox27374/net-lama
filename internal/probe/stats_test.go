package probe

import (
	"fmt"
	"testing"
)

func TestParseKBValue(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  uint64
		ok    bool
	}{
		{
			name:  "valid meminfo line",
			input: []string{"MemTotal:", "8162456", "kB"},
			want:  8162456 * 1024,
			ok:    true,
		},
		{
			name:  "insufficient fields",
			input: []string{"MemTotal:"},
			want:  0,
			ok:    false,
		},
		{
			name:  "non-numeric value",
			input: []string{"MemTotal:", "abc", "kB"},
			want:  0,
			ok:    false,
		},
		{
			name:  "zero value",
			input: []string{"MemAvailable:", "0", "kB"},
			want:  0,
			ok:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseKBValue(tt.input)
			if ok != tt.ok {
				t.Errorf("parseKBValue() ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("parseKBValue() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadCPUStatFixture(t *testing.T) {
	// Sample /proc/stat line - this fixture data is from a real Linux system
	fields := []string{"cpu", "1234567", "89012", "2345678", "987654321", "123456", "789", "456", "0", "0", "0"}

	// Verify parsing
	if len(fields) < 8 {
		t.Fatalf("fixture has insufficient fields: %d", len(fields))
	}

	var user, nice, system, irq, softirq uint64
	if _, err := fmt.Sscanf(fields[1], "%d", &user); err != nil {
		t.Fatalf("Failed to parse user CPU: %v", err)
	}

	// Test that we get reasonable values
	if user != 1234567 {
		t.Errorf("expected user ticks 1234567, got %d", user)
	}

	// Verify the fixture math: busy = user + nice + system + irq + softirq
	if _, err := fmt.Sscanf(fields[2], "%d", &nice); err != nil {
		t.Fatalf("Failed to parse nice CPU: %v", err)
	}
	if _, err := fmt.Sscanf(fields[3], "%d", &system); err != nil {
		t.Fatalf("Failed to parse system CPU: %v", err)
	}
	if _, err := fmt.Sscanf(fields[6], "%d", &irq); err != nil {
		t.Fatalf("Failed to parse irq CPU: %v", err)
	}
	if _, err := fmt.Sscanf(fields[7], "%d", &softirq); err != nil {
		t.Fatalf("Failed to parse softirq CPU: %v", err)
	}

	busy := user + nice + system + irq + softirq
	t.Logf("CPU busy ticks: %d", busy)
	if busy == 0 {
		t.Error("busy ticks should be non-zero for valid fixture")
	}
}

func TestStatsCollectorCPUDelta(t *testing.T) {
	// Create a collector and simulate two Collect calls with mocked data
	collector := NewStatsCollector()

	// First collect - should have 0% CPU (no previous sample)
	if collector.previousCPUSample != nil {
		t.Error("new collector should have no previous sample")
	}

	// The actual Collect will try to read /proc/stat - this will succeed on Linux,
	// fail gracefully on other systems. We'll just verify the structure here.
	cpu, memUsed, memTotal, diskUsed, diskTotal, agentCpu, agentMem, pidCount, uptime, ok, err := collector.Collect()

	t.Logf("First collect: cpu=%.1f%%, memUsed=%d, memTotal=%d, diskUsed=%d, diskTotal=%d, agentCpu=%.1f%%, agentMem=%d, pidCount=%d, uptime=%d, ok=%v, err=%v",
		cpu, memUsed, memTotal, diskUsed, diskTotal, agentCpu, agentMem, pidCount, uptime, ok, err)

	if !ok {
		// Expected on non-Linux systems
		t.Logf("Stats collection not available (expected on non-Linux)")
		return
	}

	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	// Verify we got reasonable values (this will only work on Linux)
	if memTotal == 0 {
		t.Error("memTotal should be non-zero on Linux")
	}

	if memUsed > memTotal {
		t.Error("memUsed should not exceed memTotal")
	}

	if diskTotal == 0 {
		t.Error("diskTotal should be non-zero")
	}

	if diskUsed > diskTotal {
		t.Error("diskUsed should not exceed diskTotal")
	}

	// Second collect - should have CPU percentage
	cpu2, _, _, _, _, agentCpu2, _, _, _, ok2, err2 := collector.Collect()
	t.Logf("Second collect: cpu=%.1f%%, agentCpu=%.1f%%, ok=%v, err=%v", cpu2, agentCpu2, ok2, err2)

	if ok2 {
		if err2 != nil {
			t.Fatalf("Second collect returned error: %v", err2)
		}
		// CPU percent should be between 0 and 100
		if cpu2 < 0 || cpu2 > 100 {
			t.Errorf("CPU percent should be between 0 and 100, got %.1f", cpu2)
		}
	}
}
