package contextengine

import (
	"context"
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/utility/mem"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

func TestSelectDocumentsUsesLLMCompressionWhenEnabled(t *testing.T) {
	oldFactory := rag.NewRetrieverFunc
	oldCompress := compressContextText
	oldEnabled := llmCompressionEnabled
	oldMinTokens := llmCompressionMinTokens
	defer func() {
		rag.NewRetrieverFunc = oldFactory
		rag.ResetSharedPool()
		compressContextText = oldCompress
		llmCompressionEnabled = oldEnabled
		llmCompressionMinTokens = oldMinTokens
	}()

	rag.ResetSharedPool()
	rag.NewRetrieverFunc = func(context.Context) (retrieverapi.Retriever, error) {
		return &fakeContextRetriever{
			docs: []*schema.Document{
				{
					ID:      "doc-1",
					Content: strings.Repeat("paymentservice timeout due to downstream db timeout during checkout. ", 40),
					MetaData: map[string]any{
						"title": "payment timeout runbook",
					},
				},
			},
		}, nil
	}

	llmCompressionEnabled = func(context.Context) bool { return true }
	llmCompressionMinTokens = func(context.Context) int { return 1 }

	var captured compressionRequest
	compressContextText = func(_ context.Context, req compressionRequest) (string, error) {
		captured = req
		return "- paymentservice timeout\n- downstream db timeout\n- checkout impacted", nil
	}

	result := selectDocuments(context.Background(), "payment timeout", ContextProfile{
		AllowDocs: true,
		Budget: ContextBudget{
			DocumentTokens: 32,
		},
	})

	if len(result.selected) != 1 {
		t.Fatalf("expected one selected document, got %d", len(result.selected))
	}
	if result.selected[0].CompressionLevel != "llm_compressed" {
		t.Fatalf("expected llm_compressed, got %#v", result.selected[0])
	}
	if captured.Query != "payment timeout" || captured.TargetTokens != 32 {
		t.Fatalf("unexpected compression request: %+v", captured)
	}
	if !containsNote(result.notes, "compression=llm_compressed=1") {
		t.Fatalf("expected compression note, got %v", result.notes)
	}
}

func TestSelectToolItemsFallsBackToTrimWithoutLLMCompression(t *testing.T) {
	oldEnabled := llmCompressionEnabled
	defer func() {
		llmCompressionEnabled = oldEnabled
	}()

	llmCompressionEnabled = func(context.Context) bool { return false }

	content := strings.Repeat("tool output about payment timeout and db saturation. ", 24)
	items := []ContextItem{
		{
			ID:            "tool-1",
			Title:         "metrics summary",
			Content:       content,
			TokenEstimate: mem.EstimateTokens(content),
		},
	}

	selected, dropped, _, notes := selectToolItems(context.Background(), "payment timeout", items, ContextProfile{
		MaxToolItems: 1,
		Budget: ContextBudget{
			ToolTokens: 20,
		},
	})

	if len(dropped) != 0 {
		t.Fatalf("expected no dropped tool items, got %d", len(dropped))
	}
	if len(selected) != 1 {
		t.Fatalf("expected one selected tool item, got %d", len(selected))
	}
	if selected[0].CompressionLevel != "trimmed" {
		t.Fatalf("expected trimmed tool item, got %#v", selected[0])
	}
	if !containsNote(notes, "compression=trimmed=1") {
		t.Fatalf("expected trim note, got %v", notes)
	}
}

func TestAssemblerTraceIncludesDocumentCompressionNote(t *testing.T) {
	resetLongTermMemory()
	oldFactory := rag.NewRetrieverFunc
	oldCompress := compressContextText
	oldEnabled := llmCompressionEnabled
	oldMinTokens := llmCompressionMinTokens
	defer func() {
		rag.NewRetrieverFunc = oldFactory
		rag.ResetSharedPool()
		compressContextText = oldCompress
		llmCompressionEnabled = oldEnabled
		llmCompressionMinTokens = oldMinTokens
	}()

	rag.ResetSharedPool()
	rag.NewRetrieverFunc = func(context.Context) (retrieverapi.Retriever, error) {
		return &fakeContextRetriever{
			docs: []*schema.Document{
				{
					ID:      "doc-1",
					Content: strings.Repeat("checkoutservice latency increased after dependency timeout. ", 120),
				},
			},
		}, nil
	}

	llmCompressionEnabled = func(context.Context) bool { return true }
	llmCompressionMinTokens = func(context.Context) int { return 1 }
	compressContextText = func(_ context.Context, req compressionRequest) (string, error) {
		return "- checkoutservice latency increased\n- dependency timeout observed", nil
	}

	assembler := NewAssembler()
	pkg, err := assembler.Assemble(context.Background(), ContextRequest{
		SessionID: "sess-trace",
		Mode:      "chat",
		Query:     "why is checkoutservice latency high",
	}, nil)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	found := false
	for _, stage := range pkg.Trace.Stages {
		if stage.Name != "documents" {
			continue
		}
		found = containsNote(stage.Notes, "compression=llm_compressed=1")
	}
	if !found {
		t.Fatalf("expected document compression trace note, got %+v", pkg.Trace.Stages)
	}
}

func containsNote(notes []string, target string) bool {
	for _, note := range notes {
		if note == target {
			return true
		}
	}
	return false
}
