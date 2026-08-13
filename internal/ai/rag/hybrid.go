package rag

import (
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/common"
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type HybridConfig struct {
	DenseTopK                  int      `json:"dense_top_k"`
	LexicalTopK                int      `json:"lexical_top_k"`
	FusionK                    int      `json:"fusion_k"`
	CandidateTopK              int      `json:"candidate_top_k"`
	FinalTopK                  int      `json:"final_top_k"`
	DenseWeight                float64  `json:"dense_weight"`
	LexicalWeight              float64  `json:"lexical_weight"`
	MetadataBoostEnabled       bool     `json:"metadata_boost_enabled"`
	KnowledgeFieldBoostEnabled bool     `json:"knowledge_field_boost_enabled"`
	KnowledgeDocIDBoost        int      `json:"knowledge_doc_id_boost"`
	KnowledgeTitleBoost        int      `json:"knowledge_title_boost"`
	KnowledgeTagsBoost         int      `json:"knowledge_tags_boost"`
	KnowledgeProviderBoost     int      `json:"knowledge_provider_boost"`
	KnowledgeFieldBoostCap     int      `json:"knowledge_field_boost_cap"`
	CoverageEnabled            bool     `json:"coverage_enabled"`
	CoverageMaxPositionGain    int      `json:"coverage_max_position_gain"`
	IntentRefinementEnabled    bool     `json:"intent_refinement_enabled"`
	IntentConnectors           []string `json:"intent_connectors"`
	IntentMaxTerms             int      `json:"intent_max_terms"`
	IntentPositiveBonus        int      `json:"intent_positive_bonus"`
	IntentExcludedPenalty      int      `json:"intent_excluded_penalty"`
}

func DefaultHybridConfig(ctx context.Context) HybridConfig {
	if cfg, ok := ctx.Value(ctxOverrideHybrid).(HybridConfig); ok {
		return cfg
	}
	topK := RetrieverTopK(ctx)
	cfg := HybridConfig{
		DenseTopK:               50,
		LexicalTopK:             50,
		FusionK:                 60,
		CandidateTopK:           topK,
		FinalTopK:               topK,
		DenseWeight:             1,
		LexicalWeight:           1,
		MetadataBoostEnabled:    true,
		KnowledgeDocIDBoost:     8,
		KnowledgeTitleBoost:     6,
		KnowledgeTagsBoost:      4,
		KnowledgeProviderBoost:  2,
		KnowledgeFieldBoostCap:  12,
		CoverageMaxPositionGain: 2,
		IntentConnectors:        []string{"而不是", "不需要", "不执行", "不是", "instead of"},
		IntentMaxTerms:          8,
		IntentPositiveBonus:     4,
		IntentExcludedPenalty:   8,
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
	if v, err := g.Cfg().Get(ctx, "rag.hybrid_candidate_top_k"); err == nil && v.Int() > 0 {
		cfg.CandidateTopK = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "rag.hybrid_metadata_boost_enabled"); err == nil {
		cfg.MetadataBoostEnabled = v.Bool()
	}
	if v, err := g.Cfg().Get(ctx, "rag.hybrid_dense_weight"); err == nil {
		if weight := v.Float64(); weight >= 0 {
			cfg.DenseWeight = weight
		}
	}
	if v, err := g.Cfg().Get(ctx, "rag.hybrid_lexical_weight"); err == nil {
		if weight := v.Float64(); weight >= 0 {
			cfg.LexicalWeight = weight
		}
	}
	if cfg.DenseWeight == 0 && cfg.LexicalWeight == 0 {
		cfg.DenseWeight, cfg.LexicalWeight = 1, 1
	}
	if v, err := g.Cfg().Get(ctx, "rag.knowledge_field_boost_enabled"); err == nil {
		cfg.KnowledgeFieldBoostEnabled = v.Bool()
	}
	readPositiveInt(ctx, "rag.knowledge_doc_id_boost", &cfg.KnowledgeDocIDBoost)
	readPositiveInt(ctx, "rag.knowledge_title_boost", &cfg.KnowledgeTitleBoost)
	readPositiveInt(ctx, "rag.knowledge_tags_boost", &cfg.KnowledgeTagsBoost)
	readPositiveInt(ctx, "rag.knowledge_provider_boost", &cfg.KnowledgeProviderBoost)
	readPositiveInt(ctx, "rag.knowledge_field_boost_cap", &cfg.KnowledgeFieldBoostCap)
	if v, err := g.Cfg().Get(ctx, "rag.coverage_enabled"); err == nil {
		cfg.CoverageEnabled = v.Bool()
	}
	readPositiveInt(ctx, "rag.coverage_max_position_gain", &cfg.CoverageMaxPositionGain)
	if v, err := g.Cfg().Get(ctx, "rag.intent_refinement_enabled"); err == nil {
		cfg.IntentRefinementEnabled = v.Bool()
	}
	if v, err := g.Cfg().Get(ctx, "rag.intent_connectors"); err == nil && len(v.Strings()) > 0 {
		cfg.IntentConnectors = append([]string(nil), v.Strings()...)
	}
	readPositiveInt(ctx, "rag.intent_max_terms", &cfg.IntentMaxTerms)
	readPositiveInt(ctx, "rag.intent_positive_bonus", &cfg.IntentPositiveBonus)
	readPositiveInt(ctx, "rag.intent_excluded_penalty", &cfg.IntentExcludedPenalty)
	return cfg
}

func readPositiveInt(ctx context.Context, key string, target *int) {
	if v, err := g.Cfg().Get(ctx, key); err == nil && v.Int() > 0 {
		*target = v.Int()
	}
}

type HybridTrace struct {
	DenseCount       int              `json:"dense_count"`
	LexicalCount     int              `json:"lexical_count"`
	FusedCount       int              `json:"fused_count"`
	CandidateCount   int              `json:"candidate_count"`
	TotalLatencyMs   int64            `json:"total_latency_ms"`
	DenseLatencyMs   int64            `json:"dense_latency_ms"`
	LexicalLatencyMs int64            `json:"lexical_latency_ms"`
	FusionLatencyMs  int64            `json:"fusion_latency_ms"`
	DenseOnlyHits    int              `json:"dense_only_hits"`
	LexicalOnlyHits  int              `json:"lexical_only_hits"`
	BothHits         int              `json:"both_hits"`
	DenseIDs         []string         `json:"dense_ids,omitempty"`
	LexicalIDs       []string         `json:"lexical_ids,omitempty"`
	FusionIDs        []string         `json:"fusion_ids,omitempty"`
	CandidateIDs     []string         `json:"candidate_ids,omitempty"`
	FinalIDs         []string         `json:"final_ids,omitempty"`
	Intent           QueryIntentTrace `json:"intent"`
}

type fusedDoc struct {
	doc       *schema.Document
	score     float64
	denseRank int
	lexRank   int
}

type sourceScope struct {
	enabled bool
	prefix  string
}

func HybridRetrieve(
	ctx context.Context,
	pool *RetrieverPool,
	lexicalIndex *BM25Index,
	query string,
	cfg HybridConfig,
) ([]*schema.Document, HybridTrace, error) {
	rr, _, err := pool.GetOrCreate(ctx)
	if err != nil {
		return nil, HybridTrace{}, err
	}
	return HybridRetrieveWithRetriever(ctx, rr, lexicalIndex, query, cfg)
}

func HybridRetrieveWithRetriever(
	ctx context.Context,
	rr retrieverapi.Retriever,
	lexicalIndex *BM25Index,
	query string,
	cfg HybridConfig,
) ([]*schema.Document, HybridTrace, error) {
	hybridStart := time.Now()
	var trace HybridTrace

	type denseResult struct {
		docs      []*schema.Document
		err       error
		latencyMs int64
	}
	type lexResult struct {
		hits      []BM25Hit
		latencyMs int64
	}

	denseCh := make(chan denseResult, 1)
	lexCh := make(chan lexResult, 1)

	go func() {
		start := time.Now()
		docs, err := rr.Retrieve(ctx, query, retrieverapi.WithTopK(cfg.DenseTopK))
		denseCh <- denseResult{docs: docs, err: err, latencyMs: time.Since(start).Milliseconds()}
	}()

	go func() {
		start := time.Now()
		var hits []BM25Hit
		if lexicalIndex != nil {
			searchOpts := BM25SearchOptions{MetadataWeight: 1}
			if cfg.KnowledgeFieldBoostEnabled {
				searchOpts.KnowledgeFieldBoost = map[string]float64{
					"knowledge_doc_id": float64(cfg.KnowledgeDocIDBoost),
					"title":            float64(cfg.KnowledgeTitleBoost),
					"tags":             float64(cfg.KnowledgeTagsBoost),
					"provider":         float64(cfg.KnowledgeProviderBoost),
				}
			}
			hits = lexicalIndex.SearchWithOptions(query, cfg.LexicalTopK, searchOpts)
		}
		lexCh <- lexResult{hits: hits, latencyMs: time.Since(start).Milliseconds()}
	}()

	dr := <-denseCh
	lr := <-lexCh

	trace.DenseLatencyMs = dr.latencyMs
	trace.LexicalLatencyMs = lr.latencyMs

	if dr.err != nil {
		trace.TotalLatencyMs = time.Since(hybridStart).Milliseconds()
		return nil, trace, dr.err
	}

	scope := sourceScopeFromContext(ctx)
	dr.docs = filterDocsBySourceScope(dr.docs, scope)
	lr.hits = filterBM25HitsBySourceScope(lr.hits, scope)

	trace.DenseCount = len(dr.docs)
	trace.LexicalCount = len(lr.hits)
	trace.DenseIDs = traceDocumentIDs(dr.docs)
	trace.LexicalIDs = traceBM25HitIDs(lr.hits)

	fusionStart := time.Now()
	denseWeight, lexicalWeight, err := hybridWeights(cfg)
	if err != nil {
		return nil, trace, err
	}
	fused := rrfFusionWeighted(dr.docs, lr.hits, cfg.FusionK, denseWeight, lexicalWeight)
	trace.FusionLatencyMs = time.Since(fusionStart).Milliseconds()
	trace.FusedCount = len(fused)
	trace.FusionIDs = traceFusedDocumentIDs(fused)

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

	if cfg.MetadataBoostEnabled || cfg.IntentRefinementEnabled {
		preRefinePositions := make(map[string]int, len(docs))
		for i, d := range docs {
			preRefinePositions[docFusionKey(d)] = i
		}
		intentQuery := query
		if rawQuery, ok := ctx.Value(ctxIntentQuery).(string); ok && strings.TrimSpace(rawQuery) != "" {
			intentQuery = rawQuery
		}
		docs, trace.Intent = refineRetrievedDocsWithTrace(intentQuery, docs, cfg)
		for i, d := range docs {
			if pre, ok := preRefinePositions[docFusionKey(d)]; ok && d.MetaData != nil {
				d.MetaData[metaKeyRefinePosition] = i + 1
				boost := float64(pre - i)
				if cfg.MetadataBoostEnabled && boost > 0 {
					d.MetaData[metaKeyMetadataBoost] = boost
				}
			}
		}
	}

	candidateTopK := cfg.CandidateTopK
	if candidateTopK <= 0 {
		candidateTopK = cfg.FinalTopK
	}
	if candidateTopK <= 0 {
		candidateTopK = 10
	}
	docs = trimRetrievedDocs(docs, candidateTopK)
	trace.CandidateCount = len(docs)
	trace.CandidateIDs = traceDocumentIDs(docs)
	trace.TotalLatencyMs = time.Since(hybridStart).Milliseconds()

	return docs, trace, nil
}

func traceDocumentIDs(docs []*schema.Document) []string {
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		if id := traceDocumentID(doc); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func traceBM25HitIDs(hits []BM25Hit) []string {
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		if id := strings.TrimSpace(hit.Meta["knowledge_doc_id"]); id != "" {
			ids = append(ids, id)
		} else if id := strings.TrimSpace(hit.DocID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func traceFusedDocumentIDs(docs []fusedDoc) []string {
	ids := make([]string, 0, len(docs))
	for _, item := range docs {
		if id := traceDocumentID(item.doc); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func traceDocumentID(doc *schema.Document) string {
	if doc != nil && doc.MetaData != nil {
		for _, key := range []string{"case_id", "caseid", "doc_id", "knowledge_doc_id"} {
			if value, ok := doc.MetaData[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		for _, key := range []string{"_source", "source", "file_name", "filename", "title"} {
			if value, ok := doc.MetaData[key].(string); ok {
				if id := canonicalTraceSourceID(value); id != "" {
					return id
				}
			}
		}
	}
	return canonicalTraceSourceID(docFusionKey(doc))
}

func canonicalTraceSourceID(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" {
		return ""
	}
	base := path.Base(value)
	if ext := path.Ext(base); ext != "" {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

func rrfFusion(denseDocs []*schema.Document, lexHits []BM25Hit, k int) []fusedDoc {
	return rrfFusionWeighted(denseDocs, lexHits, k, 1, 1)
}

func rrfFusionWeighted(denseDocs []*schema.Document, lexHits []BM25Hit, k int, denseWeight, lexicalWeight float64) []fusedDoc {
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
		e.score += denseWeight / (kf + float64(rank))
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
		e.score += lexicalWeight / (kf + float64(rank))
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

	for i, r := range results {
		ensureDocMeta(r.doc)
		r.doc.MetaData[metaKeyDenseRank] = r.denseRank
		r.doc.MetaData[metaKeyLexicalRank] = r.lexRank
		r.doc.MetaData[metaKeyFusionScore] = r.score
		r.doc.MetaData[metaKeyFusionPosition] = i + 1
	}

	return results
}

func hybridWeights(cfg HybridConfig) (float64, float64, error) {
	denseWeight, lexicalWeight := cfg.DenseWeight, cfg.LexicalWeight
	if denseWeight == 0 && lexicalWeight == 0 {
		return 1, 1, nil
	}
	if denseWeight < 0 || lexicalWeight < 0 {
		return 0, 0, fmt.Errorf("hybrid weights must be non-negative")
	}
	return denseWeight, lexicalWeight, nil
}

func docFusionKey(doc *schema.Document) string {
	if doc == nil || doc.MetaData == nil {
		if doc != nil {
			return strings.TrimSpace(doc.ID)
		}
		return ""
	}
	for _, key := range []string{"case_id", "caseid", "doc_id"} {
		if v, ok := doc.MetaData[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return strings.TrimSpace(doc.ID)
}

func lexHitToDoc(hit BM25Hit) *schema.Document {
	meta := make(map[string]any, len(hit.Meta))
	for k, v := range hit.Meta {
		meta[k] = v
	}
	return &schema.Document{
		ID:       hit.DocID,
		Content:  hit.Content,
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

func ensureDocMeta(doc *schema.Document) {
	if doc.MetaData == nil {
		doc.MetaData = make(map[string]any)
	}
}

func extractBM25Meta(doc *schema.Document) map[string]string {
	if doc == nil || doc.MetaData == nil {
		return nil
	}
	m := doc.MetaData
	out := make(map[string]string)
	for _, key := range []string{
		"case_id", "caseid", "doc_id", "knowledge_doc_id", "_source", "source", "source_uri",
		"file_path", "file_name", "filename", "title",
		"provider", "service", "instance_type", "destination",
	} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			out[key] = v
		}
	}
	for _, key := range []string{
		"tags",
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

func sourceScopeFromContext(ctx context.Context) sourceScope {
	userID, _ := ctx.Value(consts.CtxKeyUserID).(string)
	prefix := common.KnowledgeSourcePrefixForUser(userID)
	if prefix == common.KnowledgeSourceBase {
		return sourceScope{}
	}
	return sourceScope{enabled: true, prefix: prefix}
}

func filterDocsBySourceScope(docs []*schema.Document, scope sourceScope) []*schema.Document {
	if !scope.enabled || len(docs) == 0 {
		return docs
	}
	out := make([]*schema.Document, 0, len(docs))
	for _, doc := range docs {
		if doc == nil || sourceAllowedByScope(metadataString(doc.MetaData, "_source"), scope) {
			out = append(out, doc)
		}
	}
	return out
}

func filterBM25HitsBySourceScope(hits []BM25Hit, scope sourceScope) []BM25Hit {
	if !scope.enabled || len(hits) == 0 {
		return hits
	}
	out := make([]BM25Hit, 0, len(hits))
	for _, hit := range hits {
		if sourceAllowedByScope(strings.TrimSpace(hit.Meta["_source"]), scope) {
			out = append(out, hit)
		}
	}
	return out
}

func sourceAllowedByScope(source string, scope sourceScope) bool {
	if !scope.enabled {
		return true
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return true
	}
	if strings.HasPrefix(source, scope.prefix) {
		return true
	}
	if strings.HasPrefix(source, common.KnowledgeSourceBase+"users/") {
		return false
	}
	return true
}

func metadataString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	value, _ := meta[key].(string)
	return strings.TrimSpace(value)
}
