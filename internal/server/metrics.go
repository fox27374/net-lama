package server

import (
	"github.com/prometheus/client_golang/prometheus"

	pb "github.com/fox27374/net-lama/proto"
)

type Metrics struct {
	agentConnected *prometheus.GaugeVec
	resultsTotal   *prometheus.CounterVec
	errorsTotal    *prometheus.CounterVec

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

	return &Metrics{
		agentConnected: newGauge("agent_connected", "1 while the agent has an open control stream", "tenant", "site", "client"),
		resultsTotal:   newCounter("results_received_total", "Number of test results received", append(base, "type")...),
		errorsTotal:    newCounter("test_errors_total", "Number of failed test executions reported", append(base, "type")...),

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
	}
}

func (m *Metrics) SetConnected(tenant, site, client string, connected bool) {
	value := 0.0
	if connected {
		value = 1.0
	}
	m.agentConnected.WithLabelValues(tenant, site, client).Set(value)
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
	}
}
