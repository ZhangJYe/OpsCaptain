package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoubaoMultimodalEmbedderNormalizesEmptyText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload doubaoMultimodalEmbeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if got := payload.Input[0].Text; got != "." {
			t.Fatalf("expected normalized text '.', got %q", got)
		}
		_, _ = w.Write([]byte(`{"data":{"embedding":[0.1,0.2]}}`))
	}))
	defer server.Close()

	eb := &doubaoMultimodalEmbedder{
		model:      "doubao-embedding-vision",
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: server.Client(),
	}
	vec, err := eb.embedText(context.Background(), " \n\t ")
	if err != nil {
		t.Fatalf("embedText returned error: %v", err)
	}
	if len(vec) != 2 {
		t.Fatalf("expected vector length 2, got %d", len(vec))
	}
}
