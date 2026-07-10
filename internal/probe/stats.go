package probe

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
)

// StatsCollector gathers CPU, memory, and disk statistics from the system.
// On non-Linux systems, it returns zero-values and ok=false.
type StatsCollector struct {
	// previousCPUSample holds the prior /proc/stat snapshot for CPU delta calculation
	previousCPUSample *cpuSample
}

type cpuSample struct {
	time   time.Time
	busy   uint64 // sum of user, nice, system, irq, softirq
	total  uint64 // sum of all CPU time fields
}

// NewStatsCollector creates a new stats collector.
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{}
}

// Collect gathers the current CPU, memory, and disk stats.
// On Linux: reads /proc/stat, /proc/meminfo, and syscall.Statfs.
// On non-Linux: returns zero-values and ok=false.
// Returns: cpuPercent, memUsedBytes, memTotalBytes, diskUsedBytes, diskTotalBytes, ok, error
func (sc *StatsCollector) Collect() (float64, uint64, uint64, uint64, uint64, bool, error) {
	// Check if we're on Linux by trying to read /proc/stat
	_, err := readCPUSample()
	if err != nil {
		// Not Linux, or /proc not available; return zero-values
		return 0, 0, 0, 0, 0, false, nil
	}

	now := time.Now()
	cpuSample, err := readCPUSample()
	if err != nil {
		return 0, 0, 0, 0, 0, false, fmt.Errorf("reading CPU sample: %w", err)
	}

	var cpuPercent float64
	if sc.previousCPUSample != nil {
		// Calculate delta and CPU percentage
		timeDelta := now.Sub(sc.previousCPUSample.time).Seconds()
		if timeDelta > 0 {
			busyDelta := float64(cpuSample.busy - sc.previousCPUSample.busy)
			totalDelta := float64(cpuSample.total - sc.previousCPUSample.total)
			if totalDelta > 0 {
				cpuPercent = (busyDelta / totalDelta) * 100.0
			}
		}
	}
	// Save this sample for the next Collect call
	sc.previousCPUSample = cpuSample

	memUsed, memTotal, err := readMemory()
	if err != nil {
		return cpuPercent, 0, 0, 0, 0, false, fmt.Errorf("reading memory: %w", err)
	}

	diskUsed, diskTotal, err := readDisk()
	if err != nil {
		return cpuPercent, memUsed, memTotal, 0, 0, false, fmt.Errorf("reading disk: %w", err)
	}

	return cpuPercent, memUsed, memTotal, diskUsed, diskTotal, true, nil
}

// readCPUSample reads the aggregate CPU line from /proc/stat.
// Returns busy (user+nice+system+irq+softirq) and total (sum of all fields).
func readCPUSample() (*cpuSample, error) {
	content, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			// Format: cpu  <user> <nice> <system> <idle> <iowait> <irq> <softirq> ...
			fields := strings.Fields(line)
			if len(fields) < 8 {
				return nil, fmt.Errorf("unexpected cpu line format")
			}

			var user, nice, system, idle, iowait, irq, softirq uint64
			if _, err := fmt.Sscanf(fields[1], "%d", &user); err != nil {
				return nil, err
			}
			if _, err := fmt.Sscanf(fields[2], "%d", &nice); err != nil {
				return nil, err
			}
			if _, err := fmt.Sscanf(fields[3], "%d", &system); err != nil {
				return nil, err
			}
			if _, err := fmt.Sscanf(fields[4], "%d", &idle); err != nil {
				return nil, err
			}
			if _, err := fmt.Sscanf(fields[5], "%d", &iowait); err != nil {
				return nil, err
			}
			if _, err := fmt.Sscanf(fields[6], "%d", &irq); err != nil {
				return nil, err
			}
			if _, err := fmt.Sscanf(fields[7], "%d", &softirq); err != nil {
				return nil, err
			}

			busy := user + nice + system + irq + softirq
			total := user + nice + system + idle + iowait + irq + softirq
			return &cpuSample{
				time:  time.Now(),
				busy:  busy,
				total: total,
			}, nil
		}
	}

	return nil, fmt.Errorf("cpu line not found in /proc/stat")
}

// readMemory reads MemTotal and MemAvailable from /proc/meminfo.
// Returns used (MemTotal - MemAvailable) and total (MemTotal) in bytes.
func readMemory() (uint64, uint64, error) {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		value, ok := parseKBValue(fields)
		if !ok {
			continue
		}

		switch fields[0] {
		case "MemTotal:":
			memTotal = value
		case "MemAvailable:":
			memAvailable = value
		}
	}

	if memTotal == 0 {
		return 0, 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
	}

	used := memTotal - memAvailable
	return used, memTotal, nil
}

// parseKBValue parses the value from a /proc/meminfo line like "MemTotal:        8162456 kB"
func parseKBValue(fields []string) (uint64, bool) {
	if len(fields) < 2 {
		return 0, false
	}
	var value uint64
	if _, err := fmt.Sscanf(fields[1], "%d", &value); err != nil {
		return 0, false
	}
	// Convert from kB to bytes
	return value * 1024, true
}

// readDisk reads the root filesystem statistics using syscall.Statfs.
// Returns used and total bytes.
func readDisk() (uint64, uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0, 0, fmt.Errorf("statfs: %w", err)
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free

	return used, total, nil
}
