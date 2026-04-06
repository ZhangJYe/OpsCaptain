package tools

import "testing"

func TestNormalizeOptionalURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "whitespace", input: "   ", want: ""},
		{name: "placeholder", input: "${MCP_LOG_URL}", want: ""},
		{name: "url", input: "http://localhost:8081", want: "http://localhost:8081"},
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
