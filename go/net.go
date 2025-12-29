package main

import (
	//"time"

	//probing "github.com/prometheus-community/pro-bing"

	speedtest "github.com/showwin/speedtest-go/speedtest"
)

func getNetInfo(dataChan chan<- NetData, errChan chan<- error) {
	var speedtestClient = speedtest.New()

	serverList, _ := speedtestClient.FetchServers()
	targets, _ := serverList.FindServer([]int{})
	user, _ := speedtestClient.FetchUserInfo()

	for _, s := range targets {
		// Please make sure your host can access this test server,
		// otherwise you will get an error.
		// It is recommended to replace a server at this time
		s.PingTest(nil)
		s.DownloadTest()
		s.UploadTest()
		// Note: The unit of s.DLSpeed, s.ULSpeed is bytes per second, this is a float64.
		//fmt.Printf("Latency: %s, Download: %s, Upload: %s\n", s.Latency, s.DLSpeed, s.ULSpeed)
		//fmt.Println("User Details:", user)
		n := NetData{
			Name:    Ptr(s.Name),
			Country: Ptr(s.Country),
			Latency: Ptr(float64(s.Latency.Milliseconds())),
			Dlspeed: Ptr(float64(s.DLSpeed.Mbps())),
			Ulspeed: Ptr(float64(s.ULSpeed.Mbps())),
			Userip:  Ptr(user.IP),
			Userisp: Ptr(user.Isp),
		}

		s.Context.Reset()
		dataChan <- n
	}
}

// func pingHost(ip string) (bool, time.Duration) {
// 	isup := false
// 	pinger, err := probing.NewPinger(ip)
// 	if err != nil {
// 		return false, 0
// 	}

// 	pinger.Count = pingCount
// 	pinger.Timeout = 2 * time.Second
// 	pinger.SetPrivileged(false) // Set to unprivileged, otherwise root/admin rights are needed

// 	err = pinger.Run() // Blocks until done
// 	if err != nil {
// 		return false, 0
// 	}

// 	stats := pinger.Statistics()
// 	if stats.PacketsRecv > 0 {
// 		isup = true
// 	}

// 	return isup, time.Duration(stats.AvgRtt.Milliseconds())
// }
