package probe

import (
	"os"
	"testing"
)

func TestEnvEnabledProbe(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  bool
	}{
		{
			name:  "unset",
			key:   "TEST_PROBE_ENABLED_UNSET",
			value: "",
			want:  false,
		},
		{
			name:  "empty string",
			key:   "TEST_PROBE_ENABLED_EMPTY",
			value: "",
			want:  false,
		},
		{
			name:  "0",
			key:   "TEST_PROBE_ENABLED_0",
			value: "0",
			want:  false,
		},
		{
			name:  "false",
			key:   "TEST_PROBE_ENABLED_FALSE",
			value: "false",
			want:  false,
		},
		{
			name:  "off",
			key:   "TEST_PROBE_ENABLED_OFF",
			value: "off",
			want:  false,
		},
		{
			name:  "no",
			key:   "TEST_PROBE_ENABLED_NO",
			value: "no",
			want:  false,
		},
		{
			name:  "1",
			key:   "TEST_PROBE_ENABLED_1",
			value: "1",
			want:  true,
		},
		{
			name:  "true",
			key:   "TEST_PROBE_ENABLED_TRUE",
			value: "true",
			want:  true,
		},
		{
			name:  "placeholder ${VAR:-}",
			key:   "TEST_PROBE_PH1",
			value: "${TEST_PROBE_PH1:-}",
			want:  false,
		},
		{
			name:  "placeholder ${VAR:-default}",
			key:   "TEST_PROBE_PH2",
			value: "${TEST_PROBE_PH2:-default}",
			want:  false,
		},
		{
			name:  "placeholder ${VAR}",
			key:   "TEST_PROBE_PH3",
			value: "${TEST_PROBE_PH3}",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				os.Unsetenv(tt.key)
			} else {
				os.Setenv(tt.key, tt.value)
				defer os.Unsetenv(tt.key)
			}

			got := envEnabled(tt.key)
			if got != tt.want {
				t.Errorf("envEnabled(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
