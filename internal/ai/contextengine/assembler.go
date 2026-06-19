package contextengine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/contextcompression"
	"SuperBizAgent/internal/ai/memory"

	"github.com/cloudwego/eino/schema"
)

var (
	cmdbEnricherMu sync.RWMutex
	globalCMDBEnricher CMDBEnricher
)

func SetGlobalCMDBEnricher(e CMDBEnricher) {
	cmdbEnricherMu.Lock()
	defer cmdbEnricherMu.Unlock()
	globalCMDBEnricher = e
}

func NewAssemblerWithGlobal() *Assembler {
	a := NewAssembler()
	cmdbEnricherMu.RLock()
	e := globalCMDBEnricher
	cmdbEnricherMu.RUnlock()
	if e != nil {
		a.WithCMDBEnricher(e)
	}
	return a
}

type Assembler struct {
	resolver      *PolicyResolver
	now           func() time.Time
	historyRec    *HistoryRecaller
	toolRec       *ToolRecaller
	toolReranker  *ToolReranker
	intentRec     *IntentRecognizer
	cmdbEnricher  CMDBEnricher
}

func NewAssembler() *Assembler {
	return &Assembler{
		resolver:   NewPolicyResolver(),
		now:        time.Now,
		historyRec: NewHistoryRecaller(),
		toolRec:    NewToolRecaller(),
	}
}

func (a *Assembler) WithToolReranker(r *ToolReranker) *Assembler {
	a.toolReranker = r
	return a
}

func (a *Assembler) WithIntentRecognizer(r *IntentRecognizer) *Assembler {
	a.intentRec = r
	return a
}

func (a *Assembler) WithCMDBEnricher(e CMDBEnricher) *Assembler {
	a.cmdbEnricher = e
	return a
}

func (a *Assembler) Assemble(ctx context.Context, req ContextRequest, history []*schema.Message) (*ContextPackage, error) {
	start := a.now()
	profile := a.resolver.Resolve(ctx, req)

	trace := ContextAssemblyTrace{
		Profile: profile.Name,
		BudgetBefore: BudgetSnapshot{
			HistoryTokens:  profile.Budget.HistoryTokens,
			MemoryTokens:   profile.Budget.MemoryTokens,
			DocumentTokens: profile.Budget.DocumentTokens,
			ToolTokens:     profile.Budget.ToolTokens,
		},
	}

	if a.intentRec != nil {
		intentResult := a.intentRec.Recognize(ctx, req.Query)
		trace.Intent = &intentResult
		if !intentResult.Degraded && intentResult.Type != IntentUnknown {
			overridden := a.resolver.ResolveByProfile(ctx, req, ProfileForIntent(intentResult.Type))
			if overridden.Name != "" {
				profile = overridden
				trace.Profile = profile.Name
			}
		}
	}

	pkg := &ContextPackage{
		Request: req,
		Profile: profile,
		Query:   strings.TrimSpace(req.Query),
	}

	if profile.AllowHistory && len(history) > 0 {
		recallResult := a.historyRec.Recall(ctx, req.Query, history, profile.MaxHistoryMessages)
		trace.HistoryRecall = &recallResult

		selectedHistory, droppedHistory, usedHistory, historyNotes := selectHistory(recallResult.Messages, profile)
		pkg.HistoryMessages = selectedHistory
		trace.SourcesConsidered += len(history)
		trace.SourcesSelected += len(selectedHistory)
		trace.DroppedItems = append(trace.DroppedItems, droppedHistory...)
		trace.BudgetAfter.HistoryTokens = usedHistory
		trace.Stages = append(trace.Stages, StageTrace{
			Name:          "history",
			SelectedCount: len(selectedHistory),
			DroppedCount:  len(droppedHistory),
			Notes:         historyNotes,
			Recall: &RecallStageMetrics{
				CacheHits: recallResult.CacheHits,
				Embedded:  recallResult.Embedded,
				LatencyMs: recallResult.LatencyMs,
				Degraded:  recallResult.Degraded,
			},
		})
	}

	if profile.AllowMemory && req.SessionID != "" {
		retrieveLimit := profile.MaxMemoryItems
		if retrieveLimit < 1 {
			retrieveLimit = 1
		}
		entries := memory.GetLongTermMemory().RetrieveScoped(ctx, req.Query, retrieveLimit*3, memory.MemoryRetrievePolicy{
			ReadOnly:  true,
			ScopeRefs: memoryScopeRefs(req),
		})
		entries = rankMemoryEntries(entries, profile, a.now())
		selectedMemory, droppedMemory, usedMemory, memoryNotes := selectMemories(entries, profile, a.now())
		pkg.MemoryItems = selectedMemory
		trace.SourcesConsidered += len(entries)
		trace.SourcesSelected += len(selectedMemory)
		trace.DroppedItems = append(trace.DroppedItems, droppedMemory...)
		trace.BudgetAfter.MemoryTokens = usedMemory
		trace.Stages = append(trace.Stages, StageTrace{
			Name:          "memory",
			SelectedCount: len(selectedMemory),
			DroppedCount:  len(droppedMemory),
			Notes:         memoryNotes,
		})
	}

	if a.cmdbEnricher != nil {
		cmdbItems := a.cmdbEnricher.Enrich(ctx, req.Query)
		if len(cmdbItems) > 0 {
			pkg.DocumentItems = append(pkg.DocumentItems, cmdbItems...)
			trace.SourcesConsidered += len(cmdbItems)
			trace.SourcesSelected += len(cmdbItems)
			trace.Stages = append(trace.Stages, StageTrace{
				Name:          "cmdb",
				SelectedCount: len(cmdbItems),
				Notes:         []string{fmt.Sprintf("enriched %d CMDB items", len(cmdbItems))},
			})
		}
	}

	if profile.AllowDocs {
		docResult := selectDocuments(ctx, req.Query, profile)
		pkg.DocumentItems = append(docResult.selected, pkg.DocumentItems...)
		trace.SourcesConsidered += len(docResult.selected) + len(docResult.dropped)
		trace.SourcesSelected += len(docResult.selected)
		trace.DroppedItems = append(trace.DroppedItems, docResult.dropped...)
		trace.BudgetAfter.DocumentTokens = docResult.used
		trace.Stages = append(trace.Stages, StageTrace{
			Name:          "documents",
			SelectedCount: len(docResult.selected),
			DroppedCount:  len(docResult.dropped),
			Notes:         docResult.notes,
			Retrieval:     docResult.metrics,
		})
		if len(docResult.compressionReports) > 0 {
			trace.CompressionReports = append(trace.CompressionReports, docResult.compressionReports...)
		}
	}

	if profile.AllowToolResults && len(req.ToolItems) > 0 {
		toolRecallResult := a.toolRec.Recall(ctx, req.Query, req.ToolItems, profile.MaxToolItems)
		trace.ToolRecall = &toolRecallResult

		items := toolRecallResult.Items
		if a.toolReranker != nil {
			rerankOutcome := a.toolReranker.Rerank(ctx, req.Query, items, profile.MaxToolItems)
			trace.ToolRerank = &rerankOutcome
			if !rerankOutcome.Degraded {
				items = rerankOutcome.Items
			}
		}

		compCfg := contextcompression.LoadConfig(ctx)
		selectedTools, droppedTools, usedTools, toolNotes, compressionReports := selectToolItemsWithCompression(ctx, items, profile, req.Query, compCfg)
		pkg.ToolItems = selectedTools

		if len(compressionReports) > 0 {
			trace.CompressionReports = append(trace.CompressionReports, compressionReports...)
		}
		trace.SourcesConsidered += len(selectedTools) + len(droppedTools)
		trace.SourcesSelected += len(selectedTools)
		trace.DroppedItems = append(trace.DroppedItems, droppedTools...)
		trace.BudgetAfter.ToolTokens = usedTools
		trace.Stages = append(trace.Stages, StageTrace{
			Name:          "tool_results",
			SelectedCount: len(selectedTools),
			DroppedCount:  len(droppedTools),
			Notes:         toolNotes,
			Recall: &RecallStageMetrics{
				LatencyMs: toolRecallResult.LatencyMs,
			},
		})
	}

	if profile.Staged && len(pkg.MemoryItems) > 0 {
		pkg.HistoryMessages = append(memoryItemsAsMessages(pkg.MemoryItems), pkg.HistoryMessages...)
	}

	trace.LatencyMs = a.now().Sub(start).Milliseconds()
	pkg.Trace = trace
	return pkg, nil
}

func MemoryContext(pkg *ContextPackage) string {
	if pkg == nil || len(pkg.MemoryItems) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pkg.MemoryItems))
	for _, item := range pkg.MemoryItems {
		parts = append(parts, fmt.Sprintf("- [%s] %s", item.Title, item.Content))
	}
	return strings.Join(parts, "\n")
}
