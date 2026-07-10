package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Cloudflare runs a speed test against speed.cloudflare.com using only the
// standard library: a handful of tiny GETs for latency, then parallel
// download/upload transfers timed over a fixed measurement window.
const (
	cloudflareBaseURL = "https://speed.cloudflare.com"

	// cloudflareLatencySamples is the number of small round trips used to
	// estimate latency; the median of these is reported.
	cloudflareLatencySamples = 5

	// cloudflareMeasureWindow is how long each of the download/upload
	// measurements run for.
	cloudflareMeasureWindow = 10 * time.Second

	// cloudflareParallelism is the number of concurrent connections used
	// for download/upload so a single TCP stream doesn't cap the result.
	cloudflareParallelism = 4

	// cloudflareDownloadBytes is requested per GET. speed.cloudflare.com
	// rejects bytes over 100,000,000 with 403, so each download
	// connection re-requests in chunks of this size until the
	// measurement window closes rather than relying on one huge request.
	cloudflareDownloadBytes = 90 * 1000 * 1000
)

// Cloudflare runs a full speed test (latency, download, upload) against
// speed.cloudflare.com.
func Cloudflare(ctx context.Context) (*SpeedtestResult, error) {
	client := &http.Client{}

	latency, colo, err := cloudflareLatency(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("latency test: %w", err)
	}

	downloadMbps, err := cloudflareThroughput(ctx, client, cloudflareDirectionDownload)
	if err != nil {
		return nil, fmt.Errorf("download test: %w", err)
	}
	uploadMbps, err := cloudflareThroughput(ctx, client, cloudflareDirectionUpload)
	if err != nil {
		return nil, fmt.Errorf("upload test: %w", err)
	}

	if downloadMbps <= 0 || uploadMbps <= 0 {
		return nil, fmt.Errorf("returned an implausible measurement (download %.2f Mbps, upload %.2f Mbps)", downloadMbps, uploadMbps)
	}

	name := "Cloudflare"
	if colo != "" {
		name = fmt.Sprintf("Cloudflare (%s)", colo)
	}

	return &SpeedtestResult{
		ServerName:   name,
		LatencyMs:    float64(latency.Microseconds()) / 1000,
		DownloadMbps: downloadMbps,
		UploadMbps:   uploadMbps,
	}, nil
}

// cloudflareLatency issues cloudflareLatencySamples tiny GETs and returns
// the median round trip time, plus the colo code of the responding edge
// (best-effort, empty if not present in the response headers).
func cloudflareLatency(ctx context.Context, client *http.Client) (time.Duration, string, error) {
	samples := make([]time.Duration, 0, cloudflareLatencySamples)
	colo := ""
	for i := 0; i < cloudflareLatencySamples; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cloudflareBaseURL+"/__down?bytes=0", nil)
		if err != nil {
			return 0, "", err
		}
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return 0, "", err
		}
		elapsed := time.Since(start)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return 0, "", fmt.Errorf("unexpected status %s", resp.Status)
		}
		if colo == "" {
			colo = cloudflareColo(resp)
		}
		samples = append(samples, elapsed)
	}
	return medianDuration(samples), colo, nil
}

// cloudflareColo extracts the responding edge's colo code from the
// colo header if present (the observed header on speed.cloudflare.com
// responses), else from the CF-RAY header's suffix (format "<id>-<colo>").
func cloudflareColo(resp *http.Response) string {
	if v := resp.Header.Get("colo"); v != "" {
		return v
	}
	ray := resp.Header.Get("CF-RAY")
	for i := len(ray) - 1; i >= 0; i-- {
		if ray[i] == '-' {
			return ray[i+1:]
		}
	}
	return ""
}

type cloudflareDirection int

const (
	cloudflareDirectionDownload cloudflareDirection = iota
	cloudflareDirectionUpload
)

// cloudflareThroughput runs cloudflareParallelism connections for
// cloudflareMeasureWindow and returns the aggregate throughput in Mbps.
func cloudflareThroughput(ctx context.Context, client *http.Client, dir cloudflareDirection) (float64, error) {
	measureCtx, cancel := context.WithTimeout(ctx, cloudflareMeasureWindow)
	defer cancel()

	var total int64
	var wg sync.WaitGroup
	errCh := make(chan error, cloudflareParallelism)

	start := time.Now()
	for i := 0; i < cloudflareParallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			switch dir {
			case cloudflareDirectionDownload:
				err = cloudflareDownloadOnce(measureCtx, client, &total)
			case cloudflareDirectionUpload:
				err = cloudflareUploadOnce(measureCtx, client, &total)
			}
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if atomic.LoadInt64(&total) == 0 {
		select {
		case err := <-errCh:
			return 0, err
		default:
			return 0, fmt.Errorf("no data transferred")
		}
	}
	return mbps(total, elapsed), nil
}

// cloudflareDownloadOnce repeatedly GETs cloudflareDownloadBytes-sized
// chunks on one connection until the measurement window closes, adding
// every chunk read to total. Looping (rather than one huge request) works
// around speed.cloudflare.com rejecting bytes over 100,000,000.
func cloudflareDownloadOnce(ctx context.Context, client *http.Client, total *int64) error {
	url := fmt.Sprintf("%s/__down?bytes=%d", cloudflareBaseURL, cloudflareDownloadBytes)
	buf := make([]byte, 32*1024)
	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := cloudflareDownloadChunk(ctx, client, url, buf, total); err != nil {
			if ctx.Err() != nil {
				return nil // measurement window ended normally
			}
			return err
		}
	}
}

func cloudflareDownloadChunk(ctx context.Context, client *http.Client, url string, buf []byte, total *int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			atomic.AddInt64(total, int64(n))
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// cloudflareUploadOnce streams generated data to /__up until the
// measurement window closes, adding every chunk produced to total.
func cloudflareUploadOnce(ctx context.Context, client *http.Client, total *int64) error {
	body := &cloudflareUploadReader{ctx: ctx, total: total}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cloudflareBaseURL+"/__up", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	return nil
}

// cloudflareUploadReader is an io.Reader that generates data forever (well,
// until ctx is done, when it reports EOF so the request completes), and
// counts every byte it hands out.
type cloudflareUploadReader struct {
	ctx   context.Context
	total *int64
}

func (r *cloudflareUploadReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, io.EOF
	default:
	}
	for i := range p {
		p[i] = byte(i)
	}
	atomic.AddInt64(r.total, int64(len(p)))
	return len(p), nil
}

// mbps converts a byte count and elapsed time into megabits per second.
func mbps(bytesTransferred int64, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	bits := float64(bytesTransferred) * 8
	return bits / elapsed.Seconds() / 1e6
}

// medianDuration returns the median of a non-empty slice of durations.
func medianDuration(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), d...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}
