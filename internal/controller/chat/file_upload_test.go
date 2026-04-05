package chat

import (
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal.txt", "normal.txt"},
		{"../../../etc/passwd", "passwd"},
		{"file with spaces.md", "file_with_spaces.md"},
		{"<script>alert.js</script>.txt", "script_.txt"},
		{"", "unnamed"},
		{".", "unnamed"},
		{"hello世界.pdf", "hello__.pdf"},
		{"path/to/file.txt", "file.txt"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := sanitizeFilename(tc.input)
			if result != tc.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

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

func TestAllowedExtensions(t *testing.T) {
	allowed := []string{".md", ".txt", ".pdf", ".doc", ".docx", ".csv", ".json", ".yaml", ".yml"}
	rejected := []string{".exe", ".sh", ".bat", ".js", ".html", ".php", ".go", ".py"}

	for _, ext := range allowed {
		if !allowedExtensions[ext] {
			t.Errorf("extension %s should be allowed", ext)
		}
	}
	for _, ext := range rejected {
		if allowedExtensions[ext] {
			t.Errorf("extension %s should not be allowed", ext)
		}
	}
}

func TestAllowedExtensionList(t *testing.T) {
	list := allowedExtensionList()
	if len(list) != len(allowedExtensions) {
		t.Errorf("expected %d extensions, got %d", len(allowedExtensions), len(list))
	}
}
