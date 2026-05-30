package chat

import (
	"SuperBizAgent/internal/app"
	"testing"
)

func TestAllowedExtensionMap(t *testing.T) {
	allowed := []string{".md", ".txt", ".pdf", ".doc", ".docx", ".csv", ".json", ".yaml", ".yml"}
	rejected := []string{".exe", ".sh", ".bat", ".js", ".html", ".php", ".go", ".py"}

	for _, ext := range allowed {
		if !app.IsAllowedExtension(ext) {
			t.Errorf("extension %s should be allowed", ext)
		}
	}
	for _, ext := range rejected {
		if app.IsAllowedExtension(ext) {
			t.Errorf("extension %s should not be allowed", ext)
		}
	}
}
