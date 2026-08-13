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
	fieldDF  map[string]int
	avgDL    float64
	totalDoc int
}

type bm25Doc struct {
	id          string
	bodyTokens  []string
	fieldTokens map[string][]string
	tokenLen    int
	content     string
	meta        map[string]string
}

type BM25SearchOptions struct {
	MetadataWeight      float64
	KnowledgeFieldBoost map[string]float64
}

type BM25Hit struct {
	DocID   string
	Score   float64
	Content string
	Meta    map[string]string
}

func NewBM25Index() *BM25Index {
	return &BM25Index{
		k1:      1.2,
		b:       0.75,
		df:      make(map[string]int),
		fieldDF: make(map[string]int),
	}
}

func (idx *BM25Index) AddDocument(id string, content string, meta map[string]string) {
	bodyTokens := BM25Tokenize(content)
	fieldTokens := make(map[string][]string, len(meta))
	for key, value := range meta {
		if tokens := BM25Tokenize(value); len(tokens) > 0 {
			fieldTokens[key] = tokens
		}
	}
	doc := bm25Doc{
		id:          id,
		bodyTokens:  bodyTokens,
		fieldTokens: fieldTokens,
		tokenLen:    len(bodyTokens),
		content:     content,
		meta:        meta,
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	replaced := false
	for i := range idx.docs {
		if idx.docs[i].id != id {
			continue
		}
		idx.docs[i] = doc
		replaced = true
		break
	}
	if !replaced {
		idx.docs = append(idx.docs, doc)
	}
	idx.rebuildStatsLocked()
}

func (idx *BM25Index) Search(query string, topK int) []BM25Hit {
	return idx.SearchWithOptions(query, topK, BM25SearchOptions{MetadataWeight: 1})
}

func (idx *BM25Index) SearchWithOptions(query string, topK int, opts BM25SearchOptions) []BM25Hit {
	queryTokens := BM25Tokenize(query)
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
	fieldDF := idx.fieldDF
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
		for _, t := range doc.bodyTokens {
			tf[t]++
		}

		score := 0.0
		dl := float64(doc.tokenLen)
		for _, qt := range queryTokens {
			docFreq, ok := df[qt]
			if ok && docFreq > 0 && doc.tokenLen > 0 && avgDL > 0 {
				idf := math.Log(1 + (float64(n)-float64(docFreq)+0.5)/(float64(docFreq)+0.5))
				termFreq := float64(tf[qt])
				tfNorm := (termFreq * (k1 + 1)) / (termFreq + k1*(1-b+b*dl/avgDL))
				score += idf * tfNorm
			}
			fieldFreq := fieldDF[qt]
			if fieldFreq == 0 {
				continue
			}
			fieldIDF := math.Log(1 + (float64(n)-float64(fieldFreq)+0.5)/(float64(fieldFreq)+0.5))
			for field, tokens := range doc.fieldTokens {
				if !containsToken(tokens, qt) {
					continue
				}
				weight := opts.MetadataWeight
				if isKnowledgeField(field) {
					weight = opts.KnowledgeFieldBoost[field]
				}
				if weight > 0 {
					score += fieldIDF * weight
				}
			}
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
			DocID:   doc.id,
			Score:   r.score,
			Content: doc.content,
			Meta:    doc.meta,
		})
	}
	return hits
}

func isKnowledgeField(field string) bool {
	switch field {
	case "knowledge_doc_id", "title", "tags", "provider":
		return true
	default:
		return false
	}
}

func (idx *BM25Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.totalDoc
}

func (idx *BM25Index) rebuildStatsLocked() {
	idx.df = make(map[string]int)
	idx.fieldDF = make(map[string]int)
	idx.totalDoc = len(idx.docs)
	if idx.totalDoc == 0 {
		idx.avgDL = 0
		return
	}

	totalLen := 0
	for _, doc := range idx.docs {
		totalLen += doc.tokenLen
		seen := make(map[string]struct{})
		for _, t := range doc.bodyTokens {
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			idx.df[t]++
		}
		fieldSeen := make(map[string]struct{})
		for _, tokens := range doc.fieldTokens {
			for _, token := range tokens {
				fieldSeen[token] = struct{}{}
			}
		}
		for token := range fieldSeen {
			idx.fieldDF[token]++
		}
	}
	idx.avgDL = float64(totalLen) / float64(idx.totalDoc)
}

func containsToken(tokens []string, target string) bool {
	for _, token := range tokens {
		if token == target {
			return true
		}
	}
	return false
}

func IsCJK(r rune) bool {
	return (r >= 0x4e00 && r <= 0x9fff) || (r >= 0x3400 && r <= 0x4dbf)
}

func BM25Tokenize(text string) []string {
	lower := strings.ToLower(text)
	var tokens []string
	var buf strings.Builder
	var prevCJK rune
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		t := buf.String()
		buf.Reset()
		if len(t) < 2 {
			return
		}
		if _, stop := RetrievalStopwords[t]; stop {
			return
		}
		tokens = append(tokens, t)
		prevCJK = 0
	}
	for _, r := range lower {
		if IsCJK(r) {
			flush()
			tokens = append(tokens, string(r))
			if prevCJK != 0 {
				tokens = append(tokens, string([]rune{prevCJK, r}))
			}
			prevCJK = r
			continue
		}
		prevCJK = 0
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == '/' || r == ':' {
			buf.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}
