package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/contextengine"
	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/mem"
	"SuperBizAgent/utility/metrics"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type MemoryService struct {
	assembler *contextengine.Assembler
}

type MemoryListOptions struct {
	SessionID      string
	UserID         string
	ProjectID      string
	IncludeExpired bool
}

type MemoryPromoteOptions struct {
	Scope      mem.MemoryScope
	ScopeID    string
	Confidence float64
}

var (
	processMemoryEventFunc  = processMemoryEventWithConfiguredAgent
	newMemoryChatModel      = models.OpenAIForDeepSeekV3Quick
	memoryExtractionTimeout = loadMemoryExtractionTimeout
	memoryExtractionMaxJobs = loadMemoryExtractionMaxJobs
	memoryExtractionWait    = loadMemoryExtractionWait
	enqueueMemoryExtraction = enqueueMemoryExtractionDefault
)

const (
	defaultExtractTimeout = 1500 * time.Millisecond
	defaultExtractMaxJobs = 8
	defaultExtractWait    = 50 * time.Millisecond
)

var ErrMemoryExtractionLimited = errors.New("memory extraction concurrency queue timeout")

var (
	memoryExtractSemaphoreMu sync.Mutex
	memoryExtractSemaphore   chan struct{}
	memoryExtractSemaphoreN  int
)

func NewMemoryService() *MemoryService {
	return &MemoryService{
		assembler: contextengine.NewAssembler(),
	}
}

func (s *MemoryService) ResolveSessionID(ctx context.Context) string {
	if sessionID, ok := ctx.Value(consts.CtxKeySessionID).(string); ok && strings.TrimSpace(sessionID) != "" {
		return strings.TrimSpace(sessionID)
	}
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
		UserID:    memoryUserID(ctx),
		ProjectID: memoryProjectID(ctx),
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
		UserID:    memoryUserID(ctx),
		ProjectID: memoryProjectID(ctx),
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

func (s *MemoryService) ListMemories(ctx context.Context, opts MemoryListOptions) []*mem.MemoryEntry {
	return mem.GetLongTermMemory().List(memoryScopeRefsFromOptions(ctx, opts), opts.IncludeExpired)
}

func (s *MemoryService) DeleteMemory(ctx context.Context, id string) bool {
	return mem.GetLongTermMemory().Delete(ctx, strings.TrimSpace(id))
}

func (s *MemoryService) DisableMemory(ctx context.Context, id string) bool {
	return mem.GetLongTermMemory().Disable(ctx, strings.TrimSpace(id))
}

func (s *MemoryService) PromoteMemory(ctx context.Context, id string, opts MemoryPromoteOptions) bool {
	return mem.GetLongTermMemory().Promote(ctx, strings.TrimSpace(id), opts.Scope, opts.ScopeID, opts.Confidence)
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
	enqueued, err := enqueueMemoryExtraction(ctx, sessionID, query, summary)
	if err != nil {
		g.Log().Warningf(ctx, "[memory] enqueue extraction failed for session %s: %v", sessionID, err)
	}
	if enqueued {
		return
	}
	release, err := acquireMemoryExtractionSlot(ctx)
	if err != nil {
		metrics.ObserveMemoryExtraction("local", "queue_timeout")
		g.Log().Debugf(ctx, "[memory] skip extraction for session %s: %v", sessionID, err)
		return
	}
	go func(parent context.Context) {
		defer release()
		extractCtx, cancel := boundedMemoryContext(parent)
		defer cancel()
		report := processMemoryEventFunc(extractCtx, memoryEventFromContext(parent, sessionID, query, summary))
		metrics.ObserveMemoryExtraction("local", "consumed")
		if report != nil && len(report.Dropped) > 0 {
			g.Log().Debugf(parent, "[memory] dropped %d memory candidates for session %s", len(report.Dropped), sessionID)
		}
	}(ctx)
}

func memoryEventFromContext(ctx context.Context, sessionID, query, summary string) mem.MemoryEvent {
	traceID := ""
	if value, ok := ctx.Value(consts.CtxKeyTraceID).(string); ok {
		traceID = strings.TrimSpace(value)
	}
	return mem.MemoryEvent{
		SessionID: sessionID,
		UserID:    memoryUserID(ctx),
		ProjectID: memoryProjectID(ctx),
		Query:     query,
		Answer:    summary,
		TraceID:   traceID,
	}
}

func processMemoryEventWithConfiguredAgent(ctx context.Context, event mem.MemoryEvent) *mem.MemoryExtractionReport {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(event.SessionID) != "" {
		if _, ok := ctx.Value(consts.CtxKeySessionID).(string); !ok {
			ctx = context.WithValue(ctx, consts.CtxKeySessionID, strings.TrimSpace(event.SessionID))
		}
	}
	agent := mem.MemoryAgent(mem.NewRuleMemoryAgent())
	if loadMemoryAgentMode(ctx) == "llm" {
		if !memoryAgentLLMConfigured(ctx) {
			g.Log().Debug(ctx, "[memory] llm memory agent fallback to rule: model api key is not configured")
		} else {
			chatModel, err := newMemoryChatModel(ctx)
			if err != nil {
				g.Log().Debugf(ctx, "[memory] llm memory agent fallback to rule: %v", err)
			} else {
				agent = mem.NewLLMMemoryAgent(chatModel, mem.NewRuleMemoryAgent())
			}
		}
	}
	return mem.ProcessMemoryEventWithAgent(ctx, event, agent)
}

func boundedMemoryContext(parent context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if parent != nil {
		base = context.WithoutCancel(parent)
	}
	return context.WithTimeout(base, memoryExtractionTimeout(parent))
}

func loadMemoryAgentMode(ctx context.Context) string {
	v, err := g.Cfg().Get(ctx, "memory.agent_mode")
	if err != nil {
		return "rule"
	}
	mode := strings.ToLower(strings.TrimSpace(v.String()))
	switch mode {
	case "llm", "rule":
		return mode
	default:
		return "rule"
	}
}

func memoryAgentLLMConfigured(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "ds_quick_chat_model.api_key")
	if err != nil {
		return false
	}
	resolved, ok := common.ResolveOptionalEnv(v.String())
	return ok && !common.LooksLikePlaceholderSecret(resolved)
}

func loadMemoryExtractionTimeout(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "memory.extract_timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultExtractTimeout
}

func loadMemoryExtractionMaxJobs(ctx context.Context) int {
	v, err := g.Cfg().Get(ctx, "memory.extract_max_concurrency")
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	return defaultExtractMaxJobs
}

func loadMemoryExtractionWait(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "memory.extract_wait_timeout_ms")
	if err == nil && v.Int64() >= 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultExtractWait
}

func acquireMemoryExtractionSlot(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	maxJobs := memoryExtractionMaxJobs(ctx)
	if maxJobs <= 0 {
		return func() {}, nil
	}

	sem := getOrCreateMemoryExtractionSemaphore(maxJobs)
	waitCtx, cancel := context.WithTimeout(ctx, memoryExtractionWait(ctx))
	defer cancel()

	select {
	case sem <- struct{}{}:
		return func() {
			select {
			case <-sem:
			default:
			}
		}, nil
	case <-waitCtx.Done():
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			return nil, ErrMemoryExtractionLimited
		}
		return nil, waitCtx.Err()
	}
}

func getOrCreateMemoryExtractionSemaphore(size int) chan struct{} {
	memoryExtractSemaphoreMu.Lock()
	defer memoryExtractSemaphoreMu.Unlock()

	if memoryExtractSemaphore != nil && memoryExtractSemaphoreN == size {
		return memoryExtractSemaphore
	}
	memoryExtractSemaphore = make(chan struct{}, size)
	memoryExtractSemaphoreN = size
	return memoryExtractSemaphore
}

func refsFromContextItems(items []contextengine.ContextItem) []protocol.MemoryRef {
	if len(items) == 0 {
		return nil
	}
	refs := make([]protocol.MemoryRef, 0, len(items))
	for _, item := range items {
		refs = append(refs, protocol.MemoryRef{
			ID:         item.ID,
			Type:       item.Title,
			Scope:      item.Scope,
			Confidence: item.Confidence,
			Source:     item.SourceID,
			Provenance: item.Provenance,
		})
	}
	return refs
}

func memoryUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(consts.CtxKeyUserID).(string); ok {
		return strings.TrimSpace(userID)
	}
	return ""
}

func memoryProjectID(ctx context.Context) string {
	v, err := g.Cfg().Get(ctx, "memory.project_id")
	if err == nil {
		return strings.TrimSpace(v.String())
	}
	return ""
}

func memoryScopeRefsFromOptions(ctx context.Context, opts MemoryListOptions) []mem.MemoryScopeRef {
	refs := make([]mem.MemoryScopeRef, 0, 4)
	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID == "" {
		if current, ok := ctx.Value(consts.CtxKeySessionID).(string); ok {
			sessionID = strings.TrimSpace(current)
		}
	}
	if sessionID != "" {
		refs = append(refs, mem.MemoryScopeRef{Scope: mem.MemoryScopeSession, ScopeID: sessionID})
	}
	userID := strings.TrimSpace(opts.UserID)
	if userID == "" {
		userID = memoryUserID(ctx)
	}
	if userID != "" {
		refs = append(refs, mem.MemoryScopeRef{Scope: mem.MemoryScopeUser, ScopeID: userID})
	}
	projectID := strings.TrimSpace(opts.ProjectID)
	if projectID == "" {
		projectID = memoryProjectID(ctx)
	}
	if projectID != "" {
		refs = append(refs, mem.MemoryScopeRef{Scope: mem.MemoryScopeProject, ScopeID: projectID})
	}
	refs = append(refs, mem.MemoryScopeRef{Scope: mem.MemoryScopeGlobal, ScopeID: "global"})
	return refs
}
