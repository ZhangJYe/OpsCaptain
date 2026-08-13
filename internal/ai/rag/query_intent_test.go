package rag

import (
	"reflect"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func intentTestConfig() HybridConfig {
	return HybridConfig{
		IntentRefinementEnabled: true,
		IntentConnectors:        []string{"而不是", "不需要", "不执行", "不是", "instead of"},
		IntentMaxTerms:          8,
		IntentPositiveBonus:     4,
		IntentExcludedPenalty:   8,
	}
}

func TestParseQueryIntentExplicitContrasts(t *testing.T) {
	cfg := intentTestConfig()
	for _, test := range []struct {
		query string
		rule  string
	}{
		{query: "只想了解自动同步触发逻辑，不需要单次 sync options", rule: "contrast:不需要"},
		{query: "我只查 Helm 版本历史，暂时不执行 rollback", rule: "contrast:不执行"},
		{query: "应用业务指标，而不是 Kubernetes Metrics Server", rule: "contrast:而不是"},
		{query: "show business metrics instead of metrics server", rule: "contrast:instead of"},
	} {
		intent := ParseQueryIntent(test.query, cfg)
		if intent.Rule != test.rule || len(intent.PositiveTerms) == 0 || len(intent.ExcludedTerms) == 0 {
			t.Fatalf("query %q was not parsed: %+v", test.query, intent)
		}
	}
}

func TestParseQueryIntentDegradesAmbiguousInputs(t *testing.T) {
	cfg := intentTestConfig()
	for _, query := range []string{
		"普通 Helm 查询", "不是 rollback", "Helm 而不是", "Helm 不是不执行 rollback",
	} {
		if intent := ParseQueryIntent(query, cfg); intent.Rule != "" {
			t.Fatalf("query %q should degrade without intent: %+v", query, intent)
		}
	}
}

func TestIntentRefinementBoundedPenaltyAndTrace(t *testing.T) {
	cfg := intentTestConfig()
	docs := []*schema.Document{
		{ID: "excluded", Content: "single sync options details", MetaData: map[string]any{}},
		{ID: "positive", Content: "automatic sync trigger logic", MetaData: map[string]any{}},
		{ID: "combined", Content: "automatic sync trigger logic and sync options", MetaData: map[string]any{}},
	}
	refined, trace := refineRetrievedDocsWithTrace("automatic sync trigger logic instead of single sync options", docs, cfg)
	if !trace.Parsed || !trace.Applied || trace.PenalizedDocs != 1 {
		t.Fatalf("unexpected intent trace: %+v", trace)
	}
	if refined[0].ID == "excluded" {
		t.Fatalf("excluded-only document remained first: %+v", refined)
	}
	combined := docs[2].MetaData
	if combined[metaKeyIntentPenalty] != float64(0) || combined[metaKeyIntentBonus] == float64(0) {
		t.Fatalf("combined document must be retained without penalty: %+v", combined)
	}
	if docs[0].MetaData[metaKeyIntentPenalty].(float64) > float64(cfg.IntentExcludedPenalty) {
		t.Fatalf("intent penalty exceeded cap: %+v", docs[0].MetaData)
	}
}

func TestIntentRefinementDisabledAndTieCompatibility(t *testing.T) {
	docs := []*schema.Document{{ID: "a", Content: "same"}, {ID: "b", Content: "same"}}
	disabled := refineRetrievedDocsWithConfig("A instead of B", docs, HybridConfig{})
	if got := []string{disabled[0].ID, disabled[1].ID}; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("disabled intent changed order: %v", got)
	}
	cfg := intentTestConfig()
	enabled := refineRetrievedDocsWithConfig("target instead of excluded", docs, cfg)
	if got := []string{enabled[0].ID, enabled[1].ID}; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("stable tie order changed: %v", got)
	}
}
