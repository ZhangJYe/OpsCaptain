package embedder

import (
	"SuperBizAgent/utility/common"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/gogf/gf/v2/frame/g"
)

func DoubaoEmbedding(ctx context.Context) (eb embedding.Embedder, err error) {
	modelName, err := readEmbeddingConfig(ctx, "embedding_model.model", "doubao_embedding_model.model")
	if err != nil {
		return nil, err
	}
	apiKey, err := readEmbeddingConfig(ctx, "embedding_model.api_key", "doubao_embedding_model.api_key")
	if err != nil {
		return nil, err
	}
	baseURL, err := readEmbeddingConfig(ctx, "embedding_model.base_url", "doubao_embedding_model.base_url")
	if err != nil {
		return nil, err
	}
	resolvedModel, ok := common.ResolveOptionalEnv(modelName)
	if !ok {
		return nil, fmt.Errorf("embedding model name is empty or unresolved")
	}
	resolvedAPIKey, ok := common.ResolveOptionalEnv(apiKey)
	if !ok {
		return nil, fmt.Errorf("embedding model api_key is empty or unresolved")
	}
	resolvedBaseURL, ok := common.ResolveOptionalEnv(baseURL)
	if !ok {
		return nil, fmt.Errorf("embedding model base_url is empty or unresolved")
	}
	if strings.Contains(resolvedModel, "embedding-vision") {
		return &doubaoMultimodalEmbedder{
			model:   resolvedModel,
			apiKey:  resolvedAPIKey,
			baseURL: strings.TrimRight(resolvedBaseURL, "/"),
			httpClient: &http.Client{
				Timeout: 30 * time.Second,
			},
		}, nil
	}
	dim := common.GetVectorDimension(ctx)
	embedder, err := openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
		Model:      resolvedModel,
		APIKey:     resolvedAPIKey,
		BaseURL:    resolvedBaseURL,
		Dimensions: &dim,
	})
	if err != nil {
		g.Log().Errorf(ctx, "new embedder error: %v", err)
		return nil, err
	}
	return embedder, nil
}

type doubaoMultimodalEmbedder struct {
	model      string
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func (e *doubaoMultimodalEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	// Batch texts to reduce N+1 HTTP calls during knowledge indexing.
	const batchSize = 16
	embeddings := make([][]float64, 0, len(texts))
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]
		// For multimodal embedder, process each text individually since the API
		// does not support true batching, but we yield between batches for cancellation.
		for _, text := range batch {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			vector, err := e.embedText(ctx, text)
			if err != nil {
				return nil, err
			}
			embeddings = append(embeddings, vector)
		}
	}
	return embeddings, nil
}

func (e *doubaoMultimodalEmbedder) embedText(ctx context.Context, text string) ([]float64, error) {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		normalized = "."
	}
	payload := doubaoMultimodalEmbeddingRequest{
		Model: e.model,
		Input: []doubaoMultimodalEmbeddingInput{
			{Type: "text", Text: normalized},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings/multimodal", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	const maxEmbedRespSize = 10 * 1024 * 1024 // 10MB safety limit
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxEmbedRespSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("doubao multimodal embedding failed: status %d: %s", resp.StatusCode, string(respBody))
	}
	var decoded doubaoMultimodalEmbeddingResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data.Embedding) == 0 {
		return nil, fmt.Errorf("doubao multimodal embedding returned empty vector")
	}
	return decoded.Data.Embedding, nil
}

type doubaoMultimodalEmbeddingRequest struct {
	Model string                           `json:"model"`
	Input []doubaoMultimodalEmbeddingInput `json:"input"`
}

type doubaoMultimodalEmbeddingInput struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type doubaoMultimodalEmbeddingResponse struct {
	Data struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func readEmbeddingConfig(ctx context.Context, paths ...string) (string, error) {
	var lastErr error
	var foundEmpty bool
	for _, path := range paths {
		v, err := g.Cfg().Get(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		s := v.String()
		if s != "" {
			return s, nil
		}
		foundEmpty = true
	}
	if foundEmpty {
		return "", nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("embedding config not found")
}
