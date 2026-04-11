package tools

import "testing"

func TestNormalizeOptionalURLForPrometheusAddress(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "placeholder", input: "${PROMETHEUS_ADDRESS}", want: ""},
		{name: "value", input: "http://localhost:9090", want: "http://localhost:9090"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeOptionalURL(tc.input)
			if got != tc.want {
				t.Fatalf("normalizeOptionalURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
