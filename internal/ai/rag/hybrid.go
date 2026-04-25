package rag

import (
	"context"
	"sort"
	"strings"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type HybridConfig struct {
	DenseTopK            int
	LexicalTopK          int
	FusionK              int
	FinalTopK            int
	MetadataBoostEnabled bool
}

func DefaultHybridConfig(ctx context.Context) HybridConfig {
	cfg := HybridConfig{
		DenseTopK:            50,
		LexicalTopK:          50,
		FusionK:              60,
		FinalTopK:            RetrieverTopK(ctx),
		MetadataBoostEnabled: true,
	}
	if v, err := g.Cfg().Get(ctx, "rag.hybrid_dense_top_k"); err == nil && v.Int() > 0 {
		cfg.DenseTopK = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "rag.hybrid_lexical_top_k"); err == nil && v.Int() > 0 {
		cfg.LexicalTopK = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "rag.hybrid_fusion_k"); err == nil && v.Int() > 0 {
		cfg.FusionK = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "rag.hybrid_final_top_k"); err == nil && v.Int() > 0 {
		cfg.FinalTopK = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "rag.hybrid_metadata_boost_enabled"); err == nil {
		cfg.MetadataBoostEnabled = v.Bool()
	}
	return cfg
}

type HybridTrace struct {
	CacheKey          string `json:"cache_key,omitempty"`
	CacheHit          bool   `json:"cache_hit"`
	InitFailureCached bool   `json:"init_failure_cached"`
	InitLatencyMs     int64  `json:"init_latency_ms"`
	DenseCount        int    `json:"dense_count"`
	LexicalCount      int    `json:"lexical_count"`
	FusedCount        int    `json:"fused_count"`
	DenseLatencyMs    int64  `json:"dense_latency_ms"`
	LexicalLatencyMs  int64  `json:"lexical_latency_ms"`
	FusionLatencyMs   int64  `json:"fusion_latency_ms"`
	DenseOnlyHits     int    `json:"dense_only_hits"`
	LexicalOnlyHits   int    `json:"lexical_only_hits"`
	BothHits          int    `json:"both_hits"`
}

type fusedDoc struct {
	doc       *schema.Document
	score     float64
	denseRank int
	lexRank   int
}

func HybridRetrieve(
	ctx context.Context,
	pool *RetrieverPool,
	lexicalIndex *BM25Index,
	query string,
	cfg HybridConfig,
) ([]*schema.Document, HybridTrace, error) {
	var trace HybridTrace

	type denseResult struct {
		docs        []*schema.Document
		acquisition RetrieverAcquisition
		err         error
		latencyMs   int64
	}
	type lexResult struct {
		hits      []BM25Hit
		latencyMs int64
	}

	denseCh := make(chan denseResult, 1)
	lexCh := make(chan lexResult, 1)

	go func() {
		rr, acquisition, err := pool.GetOrCreate(ctx)
		if err != nil {
			denseCh <- denseResult{acquisition: acquisition, err: err}
			return
		}
		start := time.Now()
		docs, err := rr.Retrieve(ctx, query, retrieverapi.WithTopK(cfg.DenseTopK))
		denseCh <- denseResult{docs: docs, acquisition: acquisition, err: err, latencyMs: time.Since(start).Milliseconds()}
	}()

	go func() {
		start := time.Now()
		var hits []BM25Hit
		if lexicalIndex != nil {
			hits = lexicalIndex.Search(query, cfg.LexicalTopK)
		}
		lexCh <- lexResult{hits: hits, latencyMs: time.Since(start).Milliseconds()}
	}()

	dr := <-denseCh
	lr := <-lexCh

	trace.DenseLatencyMs = dr.latencyMs
	trace.LexicalLatencyMs = lr.latencyMs
	trace.CacheKey = dr.acquisition.CacheKey
	trace.CacheHit = dr.acquisition.CacheHit
	trace.InitFailureCached = dr.acquisition.InitFailureCached
	trace.InitLatencyMs = dr.acquisition.InitLatencyMs

	if dr.err != nil {
		return nil, trace, dr.err
	}

	trace.DenseCount = len(dr.docs)
	trace.LexicalCount = len(lr.hits)

	fusionStart := time.Now()
	fused := rrfFusion(dr.docs, lr.hits, cfg.FusionK)
	trace.FusionLatencyMs = time.Since(fusionStart).Milliseconds()
	trace.FusedCount = len(fused)

	denseOnly, lexOnly, both := 0, 0, 0
	for _, f := range fused {
		hasDense := f.denseRank > 0
		hasLex := f.lexRank > 0
		if hasDense && hasLex {
			both++
		} else if hasDense {
			denseOnly++
		} else {
			lexOnly++
		}
	}
	trace.DenseOnlyHits = denseOnly
	trace.LexicalOnlyHits = lexOnly
	trace.BothHits = both

	docs := make([]*schema.Document, 0, len(fused))
	for _, f := range fused {
		docs = append(docs, f.doc)
	}

	if cfg.MetadataBoostEnabled {
		docs = refineRetrievedDocs(query, docs)
	}

	finalTopK := cfg.FinalTopK
	if finalTopK <= 0 {
		finalTopK = 10
	}
	docs = trimRetrievedDocs(docs, finalTopK)

	return docs, trace, nil
}

func rrfFusion(denseDocs []*schema.Document, lexHits []BM25Hit, k int) []fusedDoc {
	if k <= 0 {
		k = 60
	}
	kf := float64(k)

	type entry struct {
		doc       *schema.Document
		score     float64
		denseRank int
		lexRank   int
	}
	byID := make(map[string]*entry)

	for i, doc := range denseDocs {
		if doc == nil {
			continue
		}
		id := docFusionKey(doc)
		if id == "" {
			id = doc.ID
		}
		if id == "" {
			continue
		}
		rank := i + 1
		e, ok := byID[id]
		if !ok {
			e = &entry{doc: doc}
			byID[id] = e
		}
		e.denseRank = rank
		e.score += 1.0 / (kf + float64(rank))
	}

	for i, hit := range lexHits {
		id := hit.DocID
		if id == "" {
			continue
		}
		rank := i + 1
		e, ok := byID[id]
		if !ok {
			e = &entry{doc: lexHitToDoc(hit)}
			byID[id] = e
		}
		e.lexRank = rank
		e.score += 1.0 / (kf + float64(rank))
	}

	results := make([]fusedDoc, 0, len(byID))
	for _, e := range byID {
		results = append(results, fusedDoc{
			doc:       e.doc,
			score:     e.score,
			denseRank: e.denseRank,
			lexRank:   e.lexRank,
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	return results
}

func docFusionKey(doc *schema.Document) string {
	if doc == nil || doc.MetaData == nil {
		return ""
	}
	for _, key := range []string{"case_id", "caseid", "doc_id", "_source"} {
		if v, ok := doc.MetaData[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func lexHitToDoc(hit BM25Hit) *schema.Document {
	meta := make(map[string]any, len(hit.Meta))
	for k, v := range hit.Meta {
		meta[k] = v
	}
	return &schema.Document{
		ID:       hit.DocID,
		MetaData: meta,
	}
}

func BuildBM25IndexFromDocs(docs []*schema.Document) *BM25Index {
	idx := NewBM25Index()
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		id := docFusionKey(doc)
		if id == "" {
			id = doc.ID
		}
		if id == "" {
			continue
		}
		meta := extractBM25Meta(doc)
		idx.AddDocument(id, doc.Content, meta)
	}
	return idx
}

func AddDocToBM25Index(idx *BM25Index, doc *schema.Document) {
	if idx == nil || doc == nil {
		return
	}
	id := docFusionKey(doc)
	if id == "" {
		id = doc.ID
	}
	if id == "" {
		return
	}
	meta := extractBM25Meta(doc)
	idx.AddDocument(id, doc.Content, meta)
}

func extractBM25Meta(doc *schema.Document) map[string]string {
	if doc == nil || doc.MetaData == nil {
		return nil
	}
	m := doc.MetaData
	out := make(map[string]string)
	for _, key := range []string{"service", "instance_type", "source", "destination"} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			out[key] = v
		}
	}
	for _, key := range []string{
		"service_tokens", "metric_names", "trace_operations",
		"trace_services", "log_keywords", "pod_tokens", "node_tokens",
	} {
		switch items := m[key].(type) {
		case []string:
			out[key] = strings.Join(items, " ")
		case []any:
			var parts []string
			for _, item := range items {
				if s, ok := item.(string); ok {
					parts = append(parts, s)
				}
			}
			if len(parts) > 0 {
				out[key] = strings.Join(parts, " ")
			}
		}
	}
	return out
}
