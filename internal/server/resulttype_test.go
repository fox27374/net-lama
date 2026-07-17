package server

import (
	"testing"

	pb "github.com/fox27374/net-lama/proto"
)

// TestResultTestType ensures every TestResult payload variant maps to the
// same type string the tests table and the UI's ?type= filter use. A missing
// case would store "unknown" and hide results from the per-type views.
func TestResultTestType(t *testing.T) {
	cases := map[string]*pb.TestResult{
		"ping":       {Result: &pb.TestResult_Ping{Ping: &pb.PingResult{}}},
		"dns":        {Result: &pb.TestResult_Dns{Dns: &pb.DnsResult{}}},
		"http":       {Result: &pb.TestResult_Http{Http: &pb.HttpResult{}}},
		"tcp":        {Result: &pb.TestResult_Tcp{Tcp: &pb.TcpResult{}}},
		"speedtest":  {Result: &pb.TestResult_Speedtest{Speedtest: &pb.SpeedtestResult{}}},
		"traceroute": {Result: &pb.TestResult_Traceroute{Traceroute: &pb.TracerouteResult{}}},
		"wlan_scan":  {Result: &pb.TestResult_WlanScan{WlanScan: &pb.WlanScanResult{}}},
		"wlan_sense": {Result: &pb.TestResult_WlanSense{WlanSense: &pb.WlanSenseResult{}}},
	}
	for want, res := range cases {
		if got := resultTestType(res); got != want {
			t.Errorf("resultTestType(%s) = %q, want %q", want, got, want)
		}
	}
}
