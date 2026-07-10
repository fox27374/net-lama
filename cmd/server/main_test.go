package main

import (
	"os"
	"testing"
)

func TestEnvOr(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		fallback string
		want     string
	}{
		{
			name:     "env var set",
			key:      "TEST_VAR",
			value:    "test-value",
			fallback: "default",
			want:     "test-value",
		},
		{
			name:     "env var unset",
			key:      "TEST_VAR_UNSET",
			value:    "",
			fallback: "default",
			want:     "default",
		},
		{
			name:     "empty env var",
			key:      "TEST_VAR_EMPTY",
			value:    "",
			fallback: "default",
			want:     "default",
		},
		{
			name:     "placeholder ${VAR:-}",
			key:      "TEST_VAR_PLACEHOLDER1",
			value:    "${TEST_VAR_PLACEHOLDER1:-}",
			fallback: "default",
			want:     "default",
		},
		{
			name:     "placeholder ${VAR:-default}",
			key:      "TEST_VAR_PLACEHOLDER2",
			value:    "${TEST_VAR_PLACEHOLDER2:-default}",
			fallback: "fallback",
			want:     "fallback",
		},
		{
			name:     "placeholder ${VAR}",
			key:      "TEST_VAR_PLACEHOLDER3",
			value:    "${TEST_VAR_PLACEHOLDER3}",
			fallback: "fallback",
			want:     "fallback",
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

			got := envOr(tt.key, tt.fallback)
			if got != tt.want {
				t.Errorf("envOr(%q, %q) = %q, want %q", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestEnvIntOr(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		fallback int
		want     int
	}{
		{
			name:     "env var set to valid int",
			key:      "TEST_INT_VAR",
			value:    "42",
			fallback: 100,
			want:     42,
		},
		{
			name:     "env var unset",
			key:      "TEST_INT_VAR_UNSET",
			value:    "",
			fallback: 100,
			want:     100,
		},
		{
			name:     "env var set to non-numeric",
			key:      "TEST_INT_VAR_INVALID",
			value:    "not-a-number",
			fallback: 100,
			want:     100,
		},
		{
			name:     "placeholder ${VAR:-}",
			key:      "TEST_INT_PH1",
			value:    "${TEST_INT_PH1:-}",
			fallback: 100,
			want:     100,
		},
		{
			name:     "placeholder ${VAR:-1000}",
			key:      "TEST_INT_PH2",
			value:    "${TEST_INT_PH2:-1000}",
			fallback: 100,
			want:     100,
		},
		{
			name:     "placeholder ${VAR}",
			key:      "TEST_INT_PH3",
			value:    "${TEST_INT_PH3}",
			fallback: 100,
			want:     100,
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

			got := envIntOr(tt.key, tt.fallback)
			if got != tt.want {
				t.Errorf("envIntOr(%q, %d) = %d, want %d", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestEnvEnabled(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  bool
	}{
		{
			name:  "unset",
			key:   "TEST_ENABLED_UNSET",
			value: "",
			want:  false,
		},
		{
			name:  "empty string",
			key:   "TEST_ENABLED_EMPTY",
			value: "",
			want:  false,
		},
		{
			name:  "0",
			key:   "TEST_ENABLED_0",
			value: "0",
			want:  false,
		},
		{
			name:  "false",
			key:   "TEST_ENABLED_FALSE",
			value: "false",
			want:  false,
		},
		{
			name:  "off",
			key:   "TEST_ENABLED_OFF",
			value: "off",
			want:  false,
		},
		{
			name:  "no",
			key:   "TEST_ENABLED_NO",
			value: "no",
			want:  false,
		},
		{
			name:  "1",
			key:   "TEST_ENABLED_1",
			value: "1",
			want:  true,
		},
		{
			name:  "true",
			key:   "TEST_ENABLED_TRUE",
			value: "true",
			want:  true,
		},
		{
			name:  "placeholder ${VAR:-}",
			key:   "TEST_ENABLED_PH1",
			value: "${TEST_ENABLED_PH1:-}",
			want:  false,
		},
		{
			name:  "placeholder ${VAR:-default}",
			key:   "TEST_ENABLED_PH2",
			value: "${TEST_ENABLED_PH2:-default}",
			want:  false,
		},
		{
			name:  "placeholder ${VAR}",
			key:   "TEST_ENABLED_PH3",
			value: "${TEST_ENABLED_PH3}",
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
