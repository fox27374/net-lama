package main

import "github.com/prometheus/client_golang/prometheus"

var mDlSpeed = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "net-lama",
		Subsystem: "speedtest",
		Name:      "dl_speed_mbps",
		Help:      "Download speed in Mbps",
	},
)

var mUlSpeed = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "net-lama",
		Subsystem: "speedtest",
		Name:      "ul_speed_mbps",
		Help:      "Upload speed in Mbps",
	},
)

var mLatency = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "net-lama",
		Subsystem: "speedtest",
		Name:      "latency_ms",
		Help:      "Latency in ms",
	},
)

// Register on a custom registry instead of the global one
func registerMetrics(reg *prometheus.Registry) {
	reg.MustRegister(
		mDlSpeed,
		mUlSpeed,
		mLatency,
	)
}
