package main

import "github.com/prometheus/client_golang/prometheus"

var mDlSpeed = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "net-lama",
		Subsystem: "speedtest",
		Name:      "dl_speed",
		Help:      "Download speed in Mbps",
	},
)

var mUlSpeed = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "net-lama",
		Subsystem: "speedtest",
		Name:      "ul_speed",
		Help:      "Upload speed in Mbps",
	},
)

// Register on a custom registry instead of the global one
func registerMetrics(reg *prometheus.Registry) {
	reg.MustRegister(
		mDlSpeed,
		mUlSpeed,
	)
}
