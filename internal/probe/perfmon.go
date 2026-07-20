// Package probe: agent-to-agent throughput test (perfmon).
//
// A hand-rolled protocol over plain TCP sockets — not iperf3-compatible,
// deliberately minimal (no external binary, no new dependency). One agent
// runs Reflector (a persistent, opt-in listener); another runs RunClient
// against its host:port. Reachability is the operator's problem, same as
// any ping/tcp/traceroute target — no discovery, no NAT traversal.
//
// Two short-lived connections, one per phase (all multi-byte integers
// big-endian):
//
//	Connection A (latency + upload):
//	  client -> server: magic(4) + version(1)             -- handshake
//	  server -> client: magic(4) + version(1)                (echoed; the
//	                                                          round trip is
//	                                                          the latency
//	                                                          sample)
//	  client -> server: phase(1)=upload + durationSeconds(4)
//	  client -> server: <raw bytes for durationSeconds, client's clock>
//	  client:           TCP half-close (CloseWrite)
//	  server -> client: byteCount(8)                       -- bytes received
//	  (both sides close A)
//
//	Connection B (download):
//	  client -> server: magic(4) + version(1)              -- handshake
//	  server -> client: magic(4) + version(1)
//	  client -> server: phase(1)=download + durationSeconds(4)
//	  server -> client: <raw bytes for durationSeconds, server's clock>
//	  server:           closes B (signals end of data to the client)
//
// Splitting upload and download across two connections lets each phase end
// on an immediate, unambiguous signal — TCP half-close or a full close —
// instead of guessing "the peer probably stopped sending" from a read
// timeout, which would either cut a slow-but-live transfer short or waste
// a fixed margin waiting one out on every run. The receiving end of each
// phase is authoritative for the byte count (matches real iperf3 practice);
// Mbps is computed against the configured duration rather than a
// re-measured stopwatch, accurate enough at these timescales and immune to
// clock-skew edge cases.
package probe

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	perfmonMagic   = "NLPM"
	perfmonVersion = byte(1)

	perfmonPhaseUpload   = byte(1)
	perfmonPhaseDownload = byte(2)

	// perfmonConnMargin is slack added to a connection's deadline beyond
	// its configured phase duration — hang protection, not used to detect
	// the end of a phase (that's CloseWrite/EOF, see the protocol above).
	perfmonConnMargin = 10 * time.Second
)

// PerfmonClientResult is the outcome of one client-side throughput test.
type PerfmonClientResult struct {
	Target          string
	Success         bool
	FailedStep      string // "connect" | "handshake" | "upload" | "download"
	LatencyMs       float64
	UploadMbps      float64
	DownloadMbps    float64
	DurationSeconds uint32
}

// RunClient runs the upload phase (connection A, also yielding the latency
// sample) then the download phase (connection B) against target.
func RunClient(ctx context.Context, target string, durationSeconds uint32) (*PerfmonClientResult, error) {
	durationSeconds = perfmonDuration(durationSeconds)
	out := &PerfmonClientResult{Target: target, DurationSeconds: durationSeconds}

	latencyMs, uploadMbps, failedStep, err := runUploadConn(ctx, target, durationSeconds)
	if err != nil {
		return out, nil
	}
	if failedStep != "" {
		out.FailedStep = failedStep
		return out, nil
	}
	out.LatencyMs = latencyMs
	out.UploadMbps = uploadMbps

	downloadMbps, failedStep, err := runDownloadConn(ctx, target, durationSeconds)
	if err != nil {
		return out, nil
	}
	if failedStep != "" {
		out.FailedStep = failedStep
		return out, nil
	}
	out.DownloadMbps = downloadMbps

	out.Success = true
	return out, nil
}

func dial(ctx context.Context, target string, durationSeconds uint32) (net.Conn, error) {
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(time.Duration(durationSeconds)*time.Second + perfmonConnMargin))
	return conn, nil
}

// runUploadConn opens connection A: handshake (-> latency), then streams
// durationSeconds of data and reads back the server-confirmed byte count.
func runUploadConn(ctx context.Context, target string, durationSeconds uint32) (latencyMs, uploadMbps float64, failedStep string, err error) {
	conn, err := dial(ctx, target, durationSeconds)
	if err != nil {
		return 0, 0, "connect", nil
	}
	defer conn.Close()

	start := time.Now()
	if err := writeHandshake(conn); err != nil {
		return 0, 0, "handshake", nil
	}
	if err := readHandshake(conn); err != nil {
		return 0, 0, "handshake", nil
	}
	latencyMs = float64(time.Since(start).Microseconds()) / 1000

	if err := writePhaseHeader(conn, perfmonPhaseUpload, durationSeconds); err != nil {
		return latencyMs, 0, "upload", nil
	}
	if err := streamFor(conn, time.Duration(durationSeconds)*time.Second); err != nil {
		return latencyMs, 0, "upload", nil
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if err := tcpConn.CloseWrite(); err != nil {
			return latencyMs, 0, "upload", nil
		}
	}
	uploadBytes, err := readByteCount(conn)
	if err != nil {
		return latencyMs, 0, "upload", nil
	}
	return latencyMs, mbps(int64(uploadBytes), time.Duration(durationSeconds)*time.Second), "", nil
}

// runDownloadConn opens connection B: handshake, request the download
// phase, then reads until the server closes the connection.
func runDownloadConn(ctx context.Context, target string, durationSeconds uint32) (downloadMbps float64, failedStep string, err error) {
	conn, err := dial(ctx, target, durationSeconds)
	if err != nil {
		return 0, "connect", nil
	}
	defer conn.Close()

	if err := writeHandshake(conn); err != nil {
		return 0, "handshake", nil
	}
	if err := readHandshake(conn); err != nil {
		return 0, "handshake", nil
	}
	if err := writePhaseHeader(conn, perfmonPhaseDownload, durationSeconds); err != nil {
		return 0, "download", nil
	}
	downloadBytes, err := io.Copy(io.Discard, conn)
	if err != nil && downloadBytes == 0 {
		return 0, "download", nil
	}
	return mbps(downloadBytes, time.Duration(durationSeconds)*time.Second), "", nil
}

// Reflector accepts connections on addr until ctx is cancelled, serving the
// perfmon protocol on each. Intended to run for the lifetime of the agent
// process, started once when NETLAMA_PERFMON_PORT / -perfmon-port is set.
func Reflector(ctx context.Context, addr string) (net.Listener, error) {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed (ctx cancelled) or fatal accept error
			}
			go func() {
				defer conn.Close()
				handleReflectorConn(conn)
			}()
		}
	}()
	return ln, nil
}

// handleReflectorConn serves exactly one phase on one connection: a
// handshake, then either an upload (read to EOF, reply with the byte
// count) or a download (stream for the requested duration). Any error just
// ends the connection — the client sees it as a failed step, the correct,
// honest outcome.
func handleReflectorConn(conn net.Conn) {
	conn.SetDeadline(time.Now().Add(2*time.Minute + perfmonConnMargin))

	if err := readHandshake(conn); err != nil {
		return
	}
	if err := writeHandshake(conn); err != nil {
		return
	}

	phase, duration, err := readPhaseHeader(conn)
	if err != nil {
		return
	}
	conn.SetDeadline(time.Now().Add(time.Duration(duration)*time.Second + perfmonConnMargin))

	switch phase {
	case perfmonPhaseUpload:
		n, err := io.Copy(io.Discard, conn)
		if err != nil && n == 0 {
			return
		}
		writeByteCount(conn, uint64(n))
	case perfmonPhaseDownload:
		streamFor(conn, time.Duration(duration)*time.Second)
		// The deferred conn.Close() in Reflector's accept loop ends the
		// connection here, which is the client's signal that download is done.
	}
}

func writeHandshake(w io.Writer) error {
	_, err := w.Write(append([]byte(perfmonMagic), perfmonVersion))
	return err
}

func readHandshake(r io.Reader) error {
	buf := make([]byte, len(perfmonMagic)+1)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	if string(buf[:len(perfmonMagic)]) != perfmonMagic || buf[len(perfmonMagic)] != perfmonVersion {
		return fmt.Errorf("perfmon: bad handshake %x", buf)
	}
	return nil
}

func writePhaseHeader(w io.Writer, phase byte, durationSeconds uint32) error {
	buf := make([]byte, 5)
	buf[0] = phase
	binary.BigEndian.PutUint32(buf[1:], durationSeconds)
	_, err := w.Write(buf)
	return err
}

func readPhaseHeader(r io.Reader) (phase byte, durationSeconds uint32, err error) {
	buf := make([]byte, 5)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, 0, err
	}
	return buf[0], binary.BigEndian.Uint32(buf[1:]), nil
}

func writeByteCount(w io.Writer, n uint64) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, n)
	_, err := w.Write(buf)
	return err
}

func readByteCount(r io.Reader) (uint64, error) {
	buf := make([]byte, 8)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf), nil
}

// streamFor writes zero-value bytes to w until d has elapsed. TCP's own
// backpressure paces this — no explicit rate limiting is needed, and none
// is wanted: the point is to saturate the link to measure it.
func streamFor(w io.Writer, d time.Duration) error {
	buf := make([]byte, 32*1024)
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

// perfmonDuration substitutes the default per-direction test length for an
// unset (zero) duration.
func perfmonDuration(d uint32) uint32 {
	if d == 0 {
		return 5
	}
	return d
}
