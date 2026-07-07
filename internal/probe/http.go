package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"time"
)

type HTTPResult struct {
	URL            string
	StatusCode     int
	DNSMs          float64
	ConnectMs      float64
	TLSMs          float64
	TTFBMs         float64
	TotalMs        float64
	CertExpiryDays float64 // -1 for plain HTTP / unknown
	ServerIP       string
}

// HTTPCheck performs a GET request and records phase timings (DNS,
// TCP connect, TLS handshake, time-to-first-byte, total) via httptrace,
// plus the response status and the TLS certificate's remaining validity.
func HTTPCheck(ctx context.Context, url string, timeoutSeconds uint32, skipTLSVerify bool) (*HTTPResult, error) {
	if timeoutSeconds == 0 {
		timeoutSeconds = 10
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	res := &HTTPResult{URL: url, CertExpiryDays: -1}
	var dnsStart, connectStart, tlsStart, firstByte time.Time

	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:  func(httptrace.DNSDoneInfo) { res.DNSMs = msSince(dnsStart) },
		ConnectStart: func(_, _ string) {
			if connectStart.IsZero() {
				connectStart = time.Now()
			}
		},
		ConnectDone: func(_, addr string, _ error) {
			res.ConnectMs = msSince(connectStart)
			res.ServerIP = addr
		},
		TLSHandshakeStart: func() { tlsStart = time.Now() },
		TLSHandshakeDone: func(cs tls.ConnectionState, err error) {
			res.TLSMs = msSince(tlsStart)
			if err == nil && len(cs.PeerCertificates) > 0 {
				res.CertExpiryDays = time.Until(cs.PeerCertificates[0].NotAfter).Hours() / 24
			}
		},
		GotFirstResponseByte: func() { firstByte = time.Now() },
	}

	req, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(reqCtx, trace), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", "net-lama-agent")

	transport := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: skipTLSVerify},
		DisableKeepAlives: true,
	}
	client := &http.Client{Transport: transport}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Drain up to 1 MiB so the transfer (and thus total time) is realistic
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	res.StatusCode = resp.StatusCode
	res.TotalMs = msSince(start)
	if !firstByte.IsZero() {
		res.TTFBMs = float64(firstByte.Sub(start).Microseconds()) / 1000
	}
	return res, nil
}

func msSince(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return float64(time.Since(t).Microseconds()) / 1000
}
