package probe

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
)

// Step timeouts; the whole test also runs under the caller's context.
const (
	wlanActiveConnectTimeout = 25 * time.Second // associate + authenticate
	wlanActiveDHCPTimeout    = 10 * time.Second
	wlanActiveDownloadCap    = 12 * time.Second // throughput sample length
	wlanActiveRouteTable     = "4242"           // policy-routing table for the test flow
)

func wlanActiveImpl(ctx context.Context, iface string, opts WlanActiveOpts) (*WlanActiveOutcome, error) {
	out := &WlanActiveOutcome{Interface: iface, SSID: opts.SSID}

	// Remember and restore the interface mode (monitor sensors share the
	// radio with the passive sweep; the caller serializes on wlanMu).
	prevType, err := getInterfaceType(ctx, iface)
	if err != nil {
		return nil, fmt.Errorf("getting interface type: %w", err)
	}
	if prevType != "managed" {
		if err := setInterfaceType(ctx, iface, "managed"); err != nil {
			return nil, fmt.Errorf("switching %s to managed: %w", iface, err)
		}
		defer setInterfaceType(context.WithoutCancel(ctx), iface, prevType)
	}

	// Config (and optional CA cert) on disk for wpa_supplicant
	dir, err := os.MkdirTemp("", "netlama-wlanactive")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	caPath := ""
	if opts.Security == "eap-peap" && !opts.InsecureSkipVerify && opts.CACertPEM != "" {
		caPath = filepath.Join(dir, "ca.pem")
		if err := os.WriteFile(caPath, []byte(opts.CACertPEM), 0600); err != nil {
			return nil, err
		}
	}
	confPath := filepath.Join(dir, "wpa.conf")
	if err := os.WriteFile(confPath, []byte(wpaSupplicantConf(opts, caPath)), 0600); err != nil {
		return nil, err
	}

	// Associate + authenticate via wpa_supplicant, timing its events
	connectCtx, cancelConnect := context.WithTimeout(ctx, wlanActiveConnectTimeout)
	defer cancelConnect()
	cmd := exec.CommandContext(connectCtx, "wpa_supplicant", "-i", iface, "-c", confPath, "-D", "nl80211")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting wpa_supplicant: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
	}()

	// TotalMs spans supplicant start (scan begin) through the last completed
	// step — teardown and mode restore are harness overhead, not connection
	// experience, so they are excluded.
	connStart := time.Now()
	done := func() (*WlanActiveOutcome, error) {
		out.TotalMs = float64(time.Since(connStart).Microseconds()) / 1000
		return out, nil
	}
	var tryingAt, assocAt time.Time
	events := make(chan struct {
		ev    wpaEvent
		bssid string
	}, 8)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if ev, bssid := parseWpaEvent(sc.Text()); ev != wpaEventNone {
				events <- struct {
					ev    wpaEvent
					bssid string
				}{ev, bssid}
			}
		}
		close(events)
	}()

	out.FailedStep = "associate"
connect:
	for {
		select {
		case <-connectCtx.Done():
			return done() // timed out in associate or authenticate step
		case e, ok := <-events:
			if !ok {
				return done() // supplicant exited without connecting
			}
			switch e.ev {
			case wpaEventTrying:
				// SSID found — everything before this was the scan phase
				if tryingAt.IsZero() {
					tryingAt = time.Now()
					out.ScanMs = float64(tryingAt.Sub(connStart).Microseconds()) / 1000
				}
			case wpaEventAssociated:
				if assocAt.IsZero() {
					assocAt = time.Now()
					from := tryingAt
					if from.IsZero() {
						from = connStart
					}
					out.AssociateMs = float64(assocAt.Sub(from).Microseconds()) / 1000
					out.BSSID = strings.ToLower(e.bssid)
					out.FailedStep = "authenticate"
				}
			case wpaEventConnected:
				if assocAt.IsZero() { // some drivers skip the associate line
					assocAt = connStart
				}
				out.AuthenticateMs = float64(time.Since(assocAt).Microseconds()) / 1000
				break connect
			case wpaEventAssocFail:
				out.FailedStep = "associate"
				return done()
			case wpaEventAuthFail:
				out.FailedStep = "authenticate"
				return done()
			}
		}
	}

	out.RSSIdBm = iwLinkSignal(ctx, iface)

	// Disable power save on the dedicated test interface — harmless hardening;
	// note it does NOT fix the post-association DHCP delay (that's the AP/switch
	// settling upstream, not our radio sleeping — see the retransmit note below).
	exec.CommandContext(ctx, "iw", "dev", iface, "set", "power_save", "off").Run()

	// DHCP: full DISCOVER→ACK exchange, no host state touched
	out.FailedStep = "dhcp"
	dhcpStart := time.Now()
	dhcpCtx, cancelDHCP := context.WithTimeout(ctx, wlanActiveDHCPTimeout)
	defer cancelDHCP()
	// Fast retransmit: on this class of AP the switch/controller ignores the
	// first ~2-4.5s of DISCOVERs after association (client MAC not forwarding
	// upstream yet), then answers in ~50ms once ready — confirmed by capture.
	// A flat 1.5s interval quantized the result up to the next tick (ready at
	// 2.3s → reported 3.0-4.5s); 400ms catches the ready-edge within a few
	// hundred ms. 400ms x 25 covers the 10s DHCP context.
	client, err := nclient4.New(iface, nclient4.WithTimeout(400*time.Millisecond), nclient4.WithRetry(25))
	if err != nil {
		return done()
	}
	lease, err := client.Request(dhcpCtx)
	client.Close()
	if err != nil || lease == nil || lease.ACK == nil {
		return done()
	}
	out.DHCPMs = float64(time.Since(dhcpStart).Microseconds()) / 1000
	out.IP = lease.ACK.YourIPAddr.String()
	if m := lease.ACK.SubnetMask(); m != nil {
		out.Netmask = net.IP(m).String()
	}
	if routers := lease.ACK.Router(); len(routers) > 0 {
		out.Gateway = routers[0].String()
	}

	// Optional throughput download through the WLAN path
	if opts.ThroughputURL == "" {
		out.Success = true
		out.FailedStep = ""
		return done()
	}
	out.FailedStep = "throughput"
	maskBits, _ := net.IPMask(net.ParseIP(out.Netmask).To4()).Size()
	if err := setupTestRoute(ctx, iface, out.IP, maskBits, out.Gateway); err != nil {
		return done()
	}
	defer teardownTestRoute(context.WithoutCancel(ctx), iface, out.IP, maskBits)

	mbps, ms, err := downloadVia(ctx, out.IP, opts.ThroughputURL)
	if err != nil {
		return done()
	}
	out.ThroughputMbps = mbps
	out.ThroughputMs = ms
	out.Success = true
	out.FailedStep = ""
	return done()
}

// setupTestRoute assigns the leased address and installs a source-routed
// default via the WLAN gateway in a dedicated table, so only the test's
// traffic uses the wireless path and host routing stays untouched.
func setupTestRoute(ctx context.Context, iface, ip string, maskBits int, gw string) error {
	cidr := fmt.Sprintf("%s/%d", ip, maskBits)
	if err := exec.CommandContext(ctx, "ip", "addr", "add", cidr, "dev", iface).Run(); err != nil {
		return err
	}
	if gw != "" {
		if err := exec.CommandContext(ctx, "ip", "route", "add", "default", "via", gw, "dev", iface, "table", wlanActiveRouteTable).Run(); err != nil {
			return err
		}
		return exec.CommandContext(ctx, "ip", "rule", "add", "from", ip, "table", wlanActiveRouteTable).Run()
	}
	return nil
}

func teardownTestRoute(ctx context.Context, iface, ip string, maskBits int) {
	exec.CommandContext(ctx, "ip", "rule", "del", "from", ip, "table", wlanActiveRouteTable).Run()
	exec.CommandContext(ctx, "ip", "route", "flush", "table", wlanActiveRouteTable).Run()
	exec.CommandContext(ctx, "ip", "addr", "del", fmt.Sprintf("%s/%d", ip, maskBits), "dev", iface).Run()
}

// downloadVia fetches url with the local address pinned to the leased IP
// (policy routing sends it out the WLAN) and reports Mbps over the sample.
func downloadVia(ctx context.Context, localIP, url string) (mbps, ms float64, err error) {
	local := &net.TCPAddr{IP: net.ParseIP(localIP)}
	transport := &http.Transport{
		DialContext: (&net.Dialer{LocalAddr: local, Timeout: 10 * time.Second}).DialContext,
	}
	dlCtx, cancel := context.WithTimeout(ctx, wlanActiveDownloadCap)
	defer cancel()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, err
	}
	start := time.Now()
	resp, err := transport.RoundTrip(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	n, err := io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(start)
	// A deadline-terminated download is still a valid sample
	if n == 0 && err != nil && dlCtx.Err() == nil {
		return 0, 0, err
	}
	sec := elapsed.Seconds()
	if sec <= 0 {
		sec = 0.001
	}
	return float64(n) * 8 / 1e6 / sec, float64(elapsed.Microseconds()) / 1000, nil
}

// iwLinkSignal reads the current link RSSI from `iw dev <iface> link`.
func iwLinkSignal(ctx context.Context, iface string) int32 {
	outBytes, err := exec.CommandContext(ctx, "iw", "dev", iface, "link").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(outBytes), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "signal: "); ok {
			if s, err := strconv.Atoi(strings.TrimSuffix(v, " dBm")); err == nil {
				return int32(s)
			}
		}
	}
	return 0
}
