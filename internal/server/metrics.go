package server

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	pb "github.com/fox27374/net-lama/proto"
)

type Metrics struct {
	agentConnected *prometheus.GaugeVec
	agentHealth    *prometheus.GaugeVec
	resultsTotal   *prometheus.CounterVec
	errorsTotal    *prometheus.CounterVec

	agentCpuPercent      *prometheus.GaugeVec
	agentMemoryUsed      *prometheus.GaugeVec
	agentMemoryTotal     *prometheus.GaugeVec
	agentDiskUsed        *prometheus.GaugeVec
	agentDiskTotal       *prometheus.GaugeVec

	speedtestDownload *prometheus.GaugeVec
	speedtestUpload   *prometheus.GaugeVec
	speedtestLatency  *prometheus.GaugeVec

	pingRttMin *prometheus.GaugeVec
	pingRttAvg *prometheus.GaugeVec
	pingRttMax *prometheus.GaugeVec
	pingLoss   *prometheus.GaugeVec

	dnsResolveTime *prometheus.GaugeVec
	dnsSuccess     *prometheus.GaugeVec

	httpTotal    *prometheus.GaugeVec
	httpTTFB     *prometheus.GaugeVec
	httpCertDays *prometheus.GaugeVec
	httpUp       *prometheus.GaugeVec

	tcpConnect *prometheus.GaugeVec
	tcpUp      *prometheus.GaugeVec

	wlanApsVisible *prometheus.GaugeVec

	wlanStations            *prometheus.GaugeVec
	wlanChannelUtilization  *prometheus.GaugeVec

	pathRtt     *prometheus.GaugeVec
	pathHops    *prometheus.GaugeVec
	pathReached *prometheus.GaugeVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	newGauge := func(name, help string, labels ...string) *prometheus.GaugeVec {
		g := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "netlama",
			Name:      name,
			Help:      help,
		}, labels)
		reg.MustRegister(g)
		return g
	}
	newCounter := func(name, help string, labels ...string) *prometheus.CounterVec {
		c := prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "netlama",
			Name:      name,
			Help:      help,
		}, labels)
		reg.MustRegister(c)
		return c
	}

	base := []string{"tenant", "site", "client", "test"}
	labels := []string{"tenant", "site", "client"}

	return &Metrics{
		agentConnected: newGauge("agent_connected", "1 while the agent has an open control stream", "tenant", "site", "client"),
		agentHealth:    newGauge("agent_health", "Agent health status: 0=healthy, 1=degraded, 2=unhealthy, -1=unknown", "tenant", "site", "client"),
		resultsTotal:   newCounter("results_received_total", "Number of test results received", append(base, "type")...),
		errorsTotal:    newCounter("test_errors_total", "Number of failed test executions reported", append(base, "type")...),

		agentCpuPercent:  newGauge("agent_cpu_percent", "Agent CPU usage as a percentage", labels...),
		agentMemoryUsed:  newGauge("agent_memory_used_bytes", "Agent memory used in bytes", labels...),
		agentMemoryTotal: newGauge("agent_memory_total_bytes", "Agent total memory in bytes", labels...),
		agentDiskUsed:    newGauge("agent_disk_used_bytes", "Agent disk used in bytes", labels...),
		agentDiskTotal:   newGauge("agent_disk_total_bytes", "Agent total disk in bytes", labels...),

		speedtestDownload: newGauge("speedtest_download_mbps", "Last measured download speed in Mbps", base...),
		speedtestUpload:   newGauge("speedtest_upload_mbps", "Last measured upload speed in Mbps", base...),
		speedtestLatency:  newGauge("speedtest_latency_ms", "Last measured speedtest latency in ms", base...),

		pingRttMin: newGauge("ping_rtt_min_ms", "Minimum ping round-trip time in ms", append(base, "target")...),
		pingRttAvg: newGauge("ping_rtt_avg_ms", "Average ping round-trip time in ms", append(base, "target")...),
		pingRttMax: newGauge("ping_rtt_max_ms", "Maximum ping round-trip time in ms", append(base, "target")...),
		pingLoss:   newGauge("ping_packet_loss_percent", "Ping packet loss in percent", append(base, "target")...),

		dnsResolveTime: newGauge("dns_resolve_time_ms", "DNS resolve time in ms", append(base, "server", "query")...),
		dnsSuccess:     newGauge("dns_success", "1 if the last DNS query succeeded, 0 otherwise", append(base, "server", "query")...),

		httpTotal:    newGauge("http_total_ms", "Total HTTP request time in ms", append(base, "url")...),
		httpTTFB:     newGauge("http_ttfb_ms", "HTTP time to first byte in ms", append(base, "url")...),
		httpCertDays: newGauge("http_cert_expiry_days", "Days until the TLS certificate expires (-1 if not HTTPS)", append(base, "url")...),
		httpUp:       newGauge("http_up", "1 if the last HTTP check returned a 2xx/3xx status", append(base, "url")...),

		tcpConnect: newGauge("tcp_connect_ms", "TCP connect time in ms", append(base, "target")...),
		tcpUp:      newGauge("tcp_up", "1 if the last TCP connect succeeded", append(base, "target")...),

		wlanApsVisible: newGauge("wlan_aps_visible", "Number of access points seen in the last WLAN scan", append(base, "interface")...),

		wlanStations:           newGauge("wlan_stations", "Number of wireless stations detected in the last WLAN sense scan", append(base, "interface")...),
		wlanChannelUtilization: newGauge("wlan_channel_utilization_pct", "WLAN channel utilization percentage", append(base, "channel")...),

		pathRtt:     newGauge("path_rtt_ms", "Round-trip time to the traceroute destination in ms", append(base, "target")...),
		pathHops:    newGauge("path_hops", "Number of hops on the traceroute path", append(base, "target")...),
		pathReached: newGauge("path_reached", "1 if the traceroute reached its destination", append(base, "target")...),
	}
}

func (m *Metrics) SetConnected(tenant, site, client string, connected bool) {
	value := 0.0
	if connected {
		value = 1.0
	}
	m.agentConnected.WithLabelValues(tenant, site, client).Set(value)
}

// SetAgentHealth sets the health status metric for an agent.
func (m *Metrics) SetAgentHealth(tenant, site, client string, status string) {
	value := -1.0 // unknown
	switch status {
	case "healthy":
		value = 0.0
	case "degraded":
		value = 1.0
	case "unhealthy":
		value = 2.0
	}
	m.agentHealth.WithLabelValues(tenant, site, client).Set(value)
}

// RecordAgentStats updates the agent resource metrics.
func (m *Metrics) RecordAgentStats(tenant, site, client string, stats *pb.AgentStats) {
	labels := []string{tenant, site, client}
	m.agentCpuPercent.WithLabelValues(labels...).Set(stats.CpuPercent)
	m.agentMemoryUsed.WithLabelValues(labels...).Set(float64(stats.MemUsedBytes))
	m.agentMemoryTotal.WithLabelValues(labels...).Set(float64(stats.MemTotalBytes))
	m.agentDiskUsed.WithLabelValues(labels...).Set(float64(stats.DiskUsedBytes))
	m.agentDiskTotal.WithLabelValues(labels...).Set(float64(stats.DiskTotalBytes))
}

// Record updates the metrics for one received test result.
func (m *Metrics) Record(tenant, site, client string, result *pb.TestResult) {
	test := result.TestName
	base := []string{tenant, site, client, test}

	switch r := result.Result.(type) {
	case *pb.TestResult_Speedtest:
		m.resultsTotal.WithLabelValues(append(base, "speedtest")...).Inc()
		if result.Error != "" {
			m.errorsTotal.WithLabelValues(append(base, "speedtest")...).Inc()
			return
		}
		m.speedtestDownload.WithLabelValues(base...).Set(r.Speedtest.DownloadMbps)
		m.speedtestUpload.WithLabelValues(base...).Set(r.Speedtest.UploadMbps)
		m.speedtestLatency.WithLabelValues(base...).Set(r.Speedtest.LatencyMs)

	case *pb.TestResult_Ping:
		m.resultsTotal.WithLabelValues(append(base, "ping")...).Inc()
		if result.Error != "" {
			m.errorsTotal.WithLabelValues(append(base, "ping")...).Inc()
			return
		}
		labels := append(base, r.Ping.Target)
		m.pingRttMin.WithLabelValues(labels...).Set(r.Ping.MinRttMs)
		m.pingRttAvg.WithLabelValues(labels...).Set(r.Ping.AvgRttMs)
		m.pingRttMax.WithLabelValues(labels...).Set(r.Ping.MaxRttMs)
		m.pingLoss.WithLabelValues(labels...).Set(r.Ping.LossPercent)

	case *pb.TestResult_Dns:
		m.resultsTotal.WithLabelValues(append(base, "dns")...).Inc()
		if result.Error != "" {
			m.errorsTotal.WithLabelValues(append(base, "dns")...).Inc()
			return
		}
		success := 0.0
		if r.Dns.Success {
			success = 1.0
		}
		labels := append(base, r.Dns.Server, r.Dns.Query)
		m.dnsResolveTime.WithLabelValues(labels...).Set(r.Dns.ResolveTimeMs)
		m.dnsSuccess.WithLabelValues(labels...).Set(success)

	case *pb.TestResult_Http:
		m.resultsTotal.WithLabelValues(append(base, "http")...).Inc()
		labels := append(base, r.Http.Url)
		if result.Error != "" {
			m.errorsTotal.WithLabelValues(append(base, "http")...).Inc()
			m.httpUp.WithLabelValues(labels...).Set(0)
			return
		}
		up := 0.0
		if r.Http.StatusCode >= 200 && r.Http.StatusCode < 400 {
			up = 1.0
		}
		m.httpTotal.WithLabelValues(labels...).Set(r.Http.TotalMs)
		m.httpTTFB.WithLabelValues(labels...).Set(r.Http.TtfbMs)
		m.httpCertDays.WithLabelValues(labels...).Set(r.Http.CertExpiryDays)
		m.httpUp.WithLabelValues(labels...).Set(up)

	case *pb.TestResult_Tcp:
		m.resultsTotal.WithLabelValues(append(base, "tcp")...).Inc()
		labels := append(base, r.Tcp.Target)
		if result.Error != "" || !r.Tcp.Connected {
			m.errorsTotal.WithLabelValues(append(base, "tcp")...).Inc()
			m.tcpUp.WithLabelValues(labels...).Set(0)
			return
		}
		m.tcpConnect.WithLabelValues(labels...).Set(r.Tcp.ConnectMs)
		m.tcpUp.WithLabelValues(labels...).Set(1)

	case *pb.TestResult_WlanPassive:
		m.resultsTotal.WithLabelValues(append(base, "wlan_passive")...).Inc()
		if result.Error != "" {
			m.errorsTotal.WithLabelValues(append(base, "wlan_passive")...).Inc()
			return
		}
		m.wlanApsVisible.WithLabelValues(append(base, r.WlanPassive.Interface)...).Set(float64(len(r.WlanPassive.Networks)))
		m.wlanStations.WithLabelValues(append(base, r.WlanPassive.Interface)...).Set(float64(len(r.WlanPassive.Stations)))
		for _, ch := range r.WlanPassive.Channels {
			channelLabel := append(base, fmt.Sprintf("%d", ch.Channel))
			m.wlanChannelUtilization.WithLabelValues(channelLabel...).Set(ch.UtilizationPct)
		}

	case *pb.TestResult_Traceroute:
		m.resultsTotal.WithLabelValues(append(base, "traceroute")...).Inc()
		labels := append(base, r.Traceroute.Target)
		if result.Error != "" {
			m.errorsTotal.WithLabelValues(append(base, "traceroute")...).Inc()
			m.pathReached.WithLabelValues(labels...).Set(0)
			return
		}
		reached := 0.0
		if r.Traceroute.Reached {
			reached = 1.0
		}
		m.pathReached.WithLabelValues(labels...).Set(reached)
		m.pathHops.WithLabelValues(labels...).Set(float64(len(r.Traceroute.Hops)))
		m.pathRtt.WithLabelValues(labels...).Set(r.Traceroute.RttMs)
	}
}
