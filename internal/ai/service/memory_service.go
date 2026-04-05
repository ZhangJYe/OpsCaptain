package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/contextengine"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/mem"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type MemoryService struct {
	assembler *contextengine.Assembler
}

var (
	extractMemoriesFunc     = mem.ExtractMemoriesWithReport
	memoryExtractionTimeout = loadMemoryExtractionTimeout
)

const defaultExtractTimeout = 1500 * time.Millisecond

func NewMemoryService() *MemoryService {
	return &MemoryService{
		assembler: contextengine.NewAssembler(),
	}
}

func (s *MemoryService) ResolveSessionID(ctx context.Context) string {
	if userID, ok := ctx.Value(consts.CtxKeyUserID).(string); ok && userID != "" {
		return "aiops_" + userID
	}
	return mem.GenerateSessionID()
}

func (s *MemoryService) BuildContext(ctx context.Context, sessionID, query string) (string, []protocol.MemoryRef) {
	contextText, refs, _ := s.BuildContextPlan(ctx, "aiops", sessionID, query)
	return contextText, refs
}

func (s *MemoryService) BuildContextPlan(ctx context.Context, mode, sessionID, query string) (string, []protocol.MemoryRef, []string) {
	pkg, err := s.assembler.Assemble(ctx, contextengine.ContextRequest{
		SessionID: sessionID,
		Mode:      mode,
		Query:     query,
	}, nil)
	if err != nil {
		return "", nil, []string{fmt.Sprintf("context assemble failed: %v", err)}
	}
	return contextengine.MemoryContext(pkg), refsFromContextItems(pkg.MemoryItems), contextengine.TraceDetails(pkg.Trace)
}

func (s *MemoryService) BuildChatPackage(ctx context.Context, sessionID, query string, history []*schema.Message) (*contextengine.ContextPackage, []string) {
	pkg, err := s.assembler.Assemble(ctx, contextengine.ContextRequest{
		SessionID: sessionID,
		Mode:      "chat",
		Query:     query,
	}, history)
	if err != nil {
		return &contextengine.ContextPackage{
			Request: contextengine.ContextRequest{
				SessionID: sessionID,
				Mode:      "chat",
				Query:     query,
			},
			Query:           query,
			HistoryMessages: history,
		}, []string{fmt.Sprintf("context assemble failed: %v", err)}
	}
	return pkg, contextengine.TraceDetails(pkg.Trace)
}

func (s *MemoryService) InjectContext(ctx context.Context, sessionID, query string) (string, []protocol.MemoryRef) {
	memoryContext, refs := s.BuildContext(ctx, sessionID, query)
	if strings.TrimSpace(memoryContext) == "" {
		return query, refs
	}
	enriched := query + "\n\n可参考的历史上下文：\n" + memoryContext
	return enriched, refs
}

func (s *MemoryService) PersistOutcome(ctx context.Context, sessionID, query, summary string) {
	if strings.TrimSpace(summary) == "" {
		return
	}
	sessionMem := mem.GetSimpleMemory(sessionID)
	sessionMem.AddUserAssistantPair(query, summary)
	go func(parent context.Context) {
		extractCtx, cancel := boundedMemoryContext(parent)
		defer cancel()
		report := extractMemoriesFunc(extractCtx, sessionID, query, summary)
		if report != nil && len(report.Dropped) > 0 {
			g.Log().Debugf(parent, "[memory] dropped %d memory candidates for session %s", len(report.Dropped), sessionID)
		}
	}(ctx)
}

func boundedMemoryContext(parent context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if parent != nil {
		base = context.WithoutCancel(parent)
	}
	return context.WithTimeout(base, memoryExtractionTimeout(parent))
}

func loadMemoryExtractionTimeout(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "memory.extract_timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultExtractTimeout
}

func refsFromContextItems(items []contextengine.ContextItem) []protocol.MemoryRef {
	if len(items) == 0 {
		return nil
	}
	refs := make([]protocol.MemoryRef, 0, len(items))
	for _, item := range items {
		refs = append(refs, protocol.MemoryRef{
			ID:   item.ID,
			Type: item.Title,
		})
	}
	return refs
}
