package eval

import (
	"context"
	"fmt"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
)

type EinoRetrieverSearcher struct {
	inner retrieverapi.Retriever
}

func NewEinoRetrieverSearcher(inner retrieverapi.Retriever) *EinoRetrieverSearcher {
	return &EinoRetrieverSearcher{inner: inner}
}

func (s *EinoRetrieverSearcher) Search(ctx context.Context, query string, topK int) ([]RetrievedDoc, error) {
	if s == nil || s.inner == nil {
		return nil, fmt.Errorf("eino retriever is nil")
	}
	docs, err := s.inner.Retrieve(ctx, query)
	if err != nil {
		return nil, err
	}
	limit := topK
	if limit <= 0 || limit > len(docs) {
		limit = len(docs)
	}
	results := make([]RetrievedDoc, 0, limit)
	for _, doc := range docs[:limit] {
		if doc == nil {
			continue
		}
		results = append(results, RetrievedDoc{
			ID:      doc.ID,
			Title:   metadataTitle(doc.MetaData),
			Content: doc.Content,
			Score:   doc.Score(),
		})
	}
	return results, nil
}

func metadataTitle(meta map[string]any) string {
	for _, key := range []string{"title", "file_name", "filename", "source"} {
		value, ok := meta[key].(string)
		if ok && value != "" {
			return value
		}
	}
	return ""
}
