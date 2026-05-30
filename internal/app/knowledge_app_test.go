package app

import "testing"

func TestIsAllowedMIME(t *testing.T) {
	tests := []struct {
		mime    string
		allowed bool
	}{
		{"text/plain", true},
		{"text/markdown", true},
		{"application/pdf", true},
		{"application/json", true},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", true},
		{"application/msword", true},
		{"image/png", false},
		{"application/x-executable", false},
		{"application/javascript", false},
	}

	for _, tc := range tests {
		t.Run(tc.mime, func(t *testing.T) {
			result := isAllowedMIME(tc.mime)
			if result != tc.allowed {
				t.Errorf("isAllowedMIME(%q) = %v, want %v", tc.mime, result, tc.allowed)
			}
		})
	}
}

func TestAllowedExtensionList(t *testing.T) {
	list := allowedExtensionList()
	if len(list) != len(allowedExtensions) {
		t.Errorf("expected %d extensions, got %d", len(allowedExtensions), len(list))
	}
}
