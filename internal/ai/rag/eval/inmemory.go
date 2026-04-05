package eval

import (
	"context"
	"sort"
	"strings"
	"unicode"
)

type InMemoryRetriever struct {
	docs []RetrievedDoc
}

func NewInMemoryRetriever(docs []RetrievedDoc) *InMemoryRetriever {
	copied := make([]RetrievedDoc, len(docs))
	copy(copied, docs)
	return &InMemoryRetriever{docs: copied}
}

func (r *InMemoryRetriever) Search(_ context.Context, query string, topK int) ([]RetrievedDoc, error) {
	if topK <= 0 {
		topK = len(r.docs)
	}
	type scoredDoc struct {
		doc   RetrievedDoc
		score float64
	}
	scored := make([]scoredDoc, 0, len(r.docs))
	for _, doc := range r.docs {
		score := lexicalScore(query, doc.Title+" "+doc.Content)
		next := doc
		next.Score = score
		scored = append(scored, scoredDoc{doc: next, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].doc.ID < scored[j].doc.ID
		}
		return scored[i].score > scored[j].score
	})

	limit := topK
	if limit > len(scored) {
		limit = len(scored)
	}
	results := make([]RetrievedDoc, 0, limit)
	for _, item := range scored[:limit] {
		results = append(results, item.doc)
	}
	return results, nil
}

func lexicalScore(query, text string) float64 {
	queryTokens := tokenSet(query)
	docTokens := tokenSet(text)

	score := 0.0
	for token := range queryTokens {
		if _, ok := docTokens[token]; ok {
			score += 1
		}
	}
	if strings.Contains(strings.ToLower(text), strings.ToLower(query)) {
		score += 2
	}
	return score
}

func tokenSet(text string) map[string]struct{} {
	tokens := make(map[string]struct{})
	var ascii strings.Builder

	flushASCII := func() {
		if ascii.Len() == 0 {
			return
		}
		tokens[ascii.String()] = struct{}{}
		ascii.Reset()
	}

	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.Is(unicode.Han, r):
			flushASCII()
			tokens[string(r)] = struct{}{}
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			ascii.WriteRune(r)
		default:
			flushASCII()
		}
	}
	flushASCII()
	return tokens
}
