package probe

import (
	"net/http"
	"testing"
	"time"
)

func TestMbps(t *testing.T) {
	cases := []struct {
		bytes   int64
		elapsed time.Duration
		want    float64
	}{
		{0, time.Second, 0},
		{1_000_000, time.Second, 8}, // 1,000,000 bytes = 8,000,000 bits = 8 Mbps
		{12_500_000, time.Second, 100},
		{1_000_000, 0, 0}, // no elapsed time -> avoid divide by zero
		{1_000_000, 2 * time.Second, 4},
	}
	for _, c := range cases {
		got := mbps(c.bytes, c.elapsed)
		if got != c.want {
			t.Errorf("mbps(%d, %v) = %v, want %v", c.bytes, c.elapsed, got, c.want)
		}
	}
}

func TestMedianDuration(t *testing.T) {
	cases := []struct {
		name string
		in   []time.Duration
		want time.Duration
	}{
		{"empty", nil, 0},
		{"single", []time.Duration{5 * time.Millisecond}, 5 * time.Millisecond},
		{
			"odd unsorted",
			[]time.Duration{30 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond, 50 * time.Millisecond, 40 * time.Millisecond},
			30 * time.Millisecond,
		},
		{
			"does not mutate input",
			[]time.Duration{3, 1, 2},
			2,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := append([]time.Duration(nil), c.in...)
			got := medianDuration(in)
			if got != c.want {
				t.Errorf("medianDuration(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestCloudflareColo(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"colo header preferred", map[string]string{"colo": "SEA", "CF-RAY": "abcdef0123456789-LAX"}, "SEA"},
		{"falls back to CF-RAY suffix", map[string]string{"CF-RAY": "abcdef0123456789-LAX"}, "LAX"},
		{"no headers", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			for k, v := range c.headers {
				resp.Header.Set(k, v)
			}
			if got := cloudflareColo(resp); got != c.want {
				t.Errorf("cloudflareColo() = %q, want %q", got, c.want)
			}
		})
	}
}
