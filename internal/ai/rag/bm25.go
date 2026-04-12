package rag

import (
	"math"
	"sort"
	"strings"
	"sync"
)

type BM25Index struct {
	mu       sync.RWMutex
	k1       float64
	b        float64
	docs     []bm25Doc
	df       map[string]int
	avgDL    float64
	totalDoc int
}

type bm25Doc struct {
	id       string
	tokens   []string
	tokenLen int
	meta     map[string]string
}

type BM25Hit struct {
	DocID string
	Score float64
	Meta  map[string]string
}

func NewBM25Index() *BM25Index {
	return &BM25Index{
		k1: 1.2,
		b:  0.75,
		df: make(map[string]int),
	}
}

func (idx *BM25Index) AddDocument(id string, content string, meta map[string]string) {
	tokens := bm25Tokenize(content)
	if meta != nil {
		for _, v := range meta {
			tokens = append(tokens, bm25Tokenize(v)...)
		}
	}
	doc := bm25Doc{
		id:       id,
		tokens:   tokens,
		tokenLen: len(tokens),
		meta:     meta,
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.docs = append(idx.docs, doc)
	seen := make(map[string]struct{})
	for _, t := range tokens {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		idx.df[t]++
	}
	idx.totalDoc++
	idx.avgDL = (idx.avgDL*float64(idx.totalDoc-1) + float64(doc.tokenLen)) / float64(idx.totalDoc)
}

func (idx *BM25Index) Search(query string, topK int) []BM25Hit {
	queryTokens := bm25Tokenize(query)
	if len(queryTokens) == 0 || topK <= 0 {
		return nil
	}

	idx.mu.RLock()
	n := idx.totalDoc
	avgDL := idx.avgDL
	k1 := idx.k1
	b := idx.b
	docs := idx.docs
	df := idx.df
	idx.mu.RUnlock()

	if n == 0 {
		return nil
	}

	type scored struct {
		idx   int
		score float64
	}
	results := make([]scored, 0, len(docs))

	for i, doc := range docs {
		tf := make(map[string]int)
		for _, t := range doc.tokens {
			tf[t]++
		}

		score := 0.0
		dl := float64(doc.tokenLen)
		for _, qt := range queryTokens {
			docFreq, ok := df[qt]
			if !ok || docFreq == 0 {
				continue
			}
			idf := math.Log(1 + (float64(n)-float64(docFreq)+0.5)/(float64(docFreq)+0.5))
			termFreq := float64(tf[qt])
			tfNorm := (termFreq * (k1 + 1)) / (termFreq + k1*(1-b+b*dl/avgDL))
			score += idf * tfNorm
		}
		if score > 0 {
			results = append(results, scored{idx: i, score: score})
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	limit := topK
	if limit > len(results) {
		limit = len(results)
	}
	hits := make([]BM25Hit, 0, limit)
	for _, r := range results[:limit] {
		doc := docs[r.idx]
		hits = append(hits, BM25Hit{
			DocID: doc.id,
			Score: r.score,
			Meta:  doc.meta,
		})
	}
	return hits
}

func (idx *BM25Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.totalDoc
}

func bm25Tokenize(text string) []string {
	lower := strings.ToLower(text)
	var tokens []string
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		t := buf.String()
		buf.Reset()
		if len(t) < 2 {
			return
		}
		if _, stop := retrievalStopwords[t]; stop {
			return
		}
		tokens = append(tokens, t)
	}
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == '/' || r == ':' {
			buf.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}
