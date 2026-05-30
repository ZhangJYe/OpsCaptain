package app

import (
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/ai/contextengine"
	"SuperBizAgent/internal/ai/events"
	"SuperBizAgent/internal/ai/memory"
	"SuperBizAgent/internal/ai/models"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/cache"
	"SuperBizAgent/utility/log_call_back"
	"SuperBizAgent/utility/resilience"
	"SuperBizAgent/utility/safety"
	traceutil "SuperBizAgent/utility/tracing"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
	"go.opentelemetry.io/otel/attribute"
)

type sessionLockEntry struct {
	mu       sync.Mutex
	refCount int
	lastUsed time.Time
}

// ChatApp orchestrates the synchronous chat business use case.
type ChatApp struct {
	sessionLocks   map[string]*sessionLockEntry
	sessionLocksMu sync.Mutex

	buildChatAgent   func(ctx context.Context, query string) (compose.Runnable[*chat_pipeline.UserMessage, *schema.Message], error)
	degradationCheck func(ctx context.Context, entrypoint string) aiservice.DegradationDecision
}

// NewChatApp creates a ChatApp with default dependencies.
func NewChatApp() *ChatApp {
	return &ChatApp{
		sessionLocks:     make(map[string]*sessionLockEntry),
		buildChatAgent:   chat_pipeline.BuildChatAgentWithQuery,
		degradationCheck: aiservice.GetDegradationDecision,
	}
}

// SetBuildChatAgent overrides the agent builder for testing.
func (a *ChatApp) SetBuildChatAgent(fn func(ctx context.Context, query string) (compose.Runnable[*chat_pipeline.UserMessage, *schema.Message], error)) {
	a.buildChatAgent = fn
}

// SetDegradationCheck overrides the degradation check for testing.
func (a *ChatApp) SetDegradationCheck(fn func(ctx context.Context, entrypoint string) aiservice.DegradationDecision) {
	a.degradationCheck = fn
}

// HandleChat executes the full synchronous chat flow.
func (a *ChatApp) HandleChat(ctx context.Context, input *ChatInput) (*ChatResult, error) {
	id := input.SessionID
	msg := input.Question
	selectedSkillIDs := chat_pipeline.NormalizeSelectedSkillIDs(input.SkillIDs)

	if err := memory.ValidateSessionID(id); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}

	requestID := guid.S()
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, id)
	ctx = context.WithValue(ctx, consts.CtxKeyRequestID, requestID)
	ctx = skills.WithSelectedSkillIDs(ctx, selectedSkillIDs)
	ctx = enrichContext(ctx, id, requestID)
	selectedSkillIDs = skills.SelectedSkillIDsFromContext(ctx)

	g.Log().Infof(ctx, "[session:%s][req:%s] Chat request received, question length: %d, selected_skills=%v", id, requestID, len(msg), selectedSkillIDs)

	decision := safety.CheckPrompt(ctx, msg)
	ctx = context.WithValue(ctx, consts.CtxKeyInjectionRiskScore, decision.RiskScore)
	ctx = context.WithValue(ctx, consts.CtxKeyInjectionRiskLevel, decision.RiskLevel)
	if !decision.Allowed {
		g.Log().Warningf(ctx, "[prompt_guard] blocked request, pattern=%s risk_score=%.2f", decision.Pattern, decision.RiskScore)
		return nil, &PromptRejectedError{
			Reason:    decision.Reason,
			RiskScore: decision.RiskScore,
			RiskLevel: decision.RiskLevel,
			Pattern:   decision.Pattern,
		}
	}
	if decision.RiskLevel == "suspicious" {
		g.Log().Warningf(ctx, "[prompt_guard] suspicious input detected, risk_score=%.2f reason=%q", decision.RiskScore, decision.Reason)
	}

	if d := a.degradationCheck(ctx, "chat"); d.Enabled {
		return &ChatResult{
			Answer:            d.Message,
			Detail:            []string{d.Reason},
			Mode:              "degraded",
			Degraded:          true,
			DegradationReason: d.Reason,
		}, nil
	}

	mu := a.acquireSessionLock(id)
	defer a.releaseSessionLock(id, mu)

	sessionMem := memory.GetSimpleMemory(id)
	bypassResponseCache := shouldBypassCache(msg)

	if !bypassResponseCache {
		if entry, found, cacheErr := cache.LoadChatResponse(ctx, id, msg, selectedSkillIDs...); cacheErr != nil {
			g.Log().Warningf(ctx, "[session:%s][req:%s] cache lookup failed: %v", id, requestID, cacheErr)
		} else if found {
			answer, detail := filterOutput(ctx, entry.Answer, entry.Detail)
			return &ChatResult{
				Answer: answer,
				Detail: detail,
				Mode:   "cache",
				Cached: true,
			}, nil
		}
	} else {
		g.Log().Debugf(ctx, "[session:%s][req:%s] bypass chat cache for lightweight social input", id, requestID)
	}

	memorySvc := aiservice.NewMemoryService()
	contextPkg, contextDetail := memorySvc.BuildChatPackage(ctx, id, msg, sessionMem.GetContextMessages())

	userMessage := &chat_pipeline.UserMessage{
		ID:        id,
		Query:     msg,
		Documents: contextengine.DocumentsContent(contextPkg),
		History:   contextPkg.HistoryMessages,
	}

	runner, err := a.buildChatAgent(ctx, msg)
	if err != nil {
		g.Log().Errorf(ctx, "[session:%s][req:%s] BuildChatAgent failed: %v", id, requestID, err)
		return nil, err
	}

	out, err := runner.Invoke(ctx, userMessage, compose.WithCallbacks(log_call_back.LogCallback(nil)))
	if err != nil {
		if status, message := classifyError(err); status != 0 {
			return &ChatResult{
				Answer:            message,
				Detail:            []string{message},
				Mode:              "degraded",
				Degraded:          true,
				DegradationReason: message,
				HTTPStatus:        status,
			}, nil
		}
		g.Log().Errorf(ctx, "[session:%s][req:%s] Agent invoke failed: %v", id, requestID, err)
		return nil, err
	}

	answer, detail := filterOutput(ctx, out.Content, contextDetail)
	memorySvc.PersistOutcome(ctx, id, msg, answer)
	if !bypassResponseCache {
		if cacheErr := cache.StoreChatResponse(ctx, id, msg, cache.ChatResponseEntry{
			Answer: answer,
			Detail: detail,
			Mode:   "chat",
		}, selectedSkillIDs...); cacheErr != nil {
			g.Log().Warningf(ctx, "[session:%s][req:%s] cache store failed: %v", id, requestID, cacheErr)
		}
	}

	g.Log().Infof(ctx, "[session:%s][req:%s] Chat completed, answer length: %d, turns: %d",
		id, requestID, len(answer), sessionMem.TurnCount())

	return &ChatResult{
		Answer: answer,
		Detail: detail,
		Mode:   "chat",
	}, nil
}

func (a *ChatApp) acquireSessionLock(id string) *sessionLockEntry {
	a.sessionLocksMu.Lock()
	entry, ok := a.sessionLocks[id]
	if !ok {
		entry = &sessionLockEntry{}
		a.sessionLocks[id] = entry
	}
	entry.refCount++
	entry.lastUsed = time.Now()
	a.sessionLocksMu.Unlock()
	entry.mu.Lock()
	return entry
}

func (a *ChatApp) releaseSessionLock(id string, entry *sessionLockEntry) {
	if entry == nil {
		return
	}
	entry.mu.Unlock()
	a.sessionLocksMu.Lock()
	defer a.sessionLocksMu.Unlock()
	current, ok := a.sessionLocks[id]
	if !ok || current != entry {
		return
	}
	if entry.refCount > 0 {
		entry.refCount--
	}
	entry.lastUsed = time.Now()
	if entry.refCount == 0 {
		delete(a.sessionLocks, id)
	}
}

func enrichContext(ctx context.Context, sessionID, requestID string) context.Context {
	traceutil.SetAttributes(
		ctx,
		attribute.String("session.id", strings.TrimSpace(sessionID)),
		attribute.String("request.id", strings.TrimSpace(requestID)),
	)
	return traceutil.ContextWithTraceID(ctx)
}

func filterOutput(ctx context.Context, content string, details []string) (string, []string) {
	filtered := safety.FilterOutput(ctx, content)
	if filtered.Redacted {
		g.Log().Warningf(ctx, "[output_filter] redacted response, reasons=%s", strings.Join(filtered.Reasons, ","))
	}
	return filtered.Content, safety.FilterDetails(ctx, details)
}

func shouldBypassCache(query string) bool {
	normalized := strings.TrimSpace(strings.ToLower(query))
	if normalized == "" {
		return false
	}
	if strings.ContainsAny(normalized, "\n\t") {
		return false
	}
	switch normalized {
	case "hi", "hello", "hey", "你好", "您好", "嗨", "哈喽", "在吗", "在么", "早", "早上好", "晚上好", "午安":
		return true
	default:
		return false
	}
}

func classifyError(err error) (int, string) {
	if aiservice.IsDailyTokenLimitError(err) {
		return 429, "daily token limit exceeded for this session"
	}
	if resilience.IsConcurrencyLimitError(err) {
		return 503, "AI is temporarily busy. Please retry shortly."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return 504, "AI response timeout. The request may still be processing — please retry."
	}
	if errors.Is(err, context.Canceled) {
		return 0, ""
	}
	if errors.Is(err, resilience.ErrCircuitBreakerOpen) {
		return 503, "AI service temporarily unavailable. Please retry later."
	}
	return 0, ""
}

func (a *ChatApp) ValidateChatInput(ctx context.Context, sessionID, question string) error {
	if err := memory.ValidateSessionID(sessionID); err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}
	decision := safety.CheckPrompt(ctx, question)
	if !decision.Allowed {
		return &PromptRejectedError{
			Reason:    decision.Reason,
			RiskScore: decision.RiskScore,
			RiskLevel: decision.RiskLevel,
			Pattern:   decision.Pattern,
		}
	}
	return nil
}

// ChatStreamInput is the application-layer input for a streaming chat request.
type ChatStreamInput struct {
	SessionID string
	Question  string
	SkillIDs  []string
}

// HandleChatStream executes the full streaming chat flow.
// The caller provides a StreamSink for sending events to the client.
func (a *ChatApp) HandleChatStream(ctx context.Context, input *ChatStreamInput, sink StreamSink) (*ChatStreamResult, error) {
	id := input.SessionID
	msg := input.Question
	selectedSkillIDs := chat_pipeline.NormalizeSelectedSkillIDs(input.SkillIDs)

	if err := memory.ValidateSessionID(id); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}

	requestID := guid.S()
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, id)
	ctx = context.WithValue(ctx, consts.CtxKeyRequestID, requestID)
	ctx = context.WithValue(ctx, consts.CtxKeyClientID, id)
	ctx = skills.WithSelectedSkillIDs(ctx, selectedSkillIDs)
	ctx = enrichContext(ctx, id, requestID)
	selectedSkillIDs = skills.SelectedSkillIDsFromContext(ctx)

	phaseStart := time.Now()
	g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream start, question length: %d, selected_skills=%v", id, requestID, len(msg), selectedSkillIDs)

	decision := safety.CheckPrompt(ctx, msg)
	ctx = context.WithValue(ctx, consts.CtxKeyInjectionRiskScore, decision.RiskScore)
	ctx = context.WithValue(ctx, consts.CtxKeyInjectionRiskLevel, decision.RiskLevel)
	if !decision.Allowed {
		g.Log().Warningf(ctx, "[prompt_guard] blocked request, pattern=%s risk_score=%.2f", decision.Pattern, decision.RiskScore)
		return nil, &PromptRejectedError{
			Reason:    decision.Reason,
			RiskScore: decision.RiskScore,
			RiskLevel: decision.RiskLevel,
			Pattern:   decision.Pattern,
		}
	}
	if decision.RiskLevel == "suspicious" {
		g.Log().Warningf(ctx, "[prompt_guard] suspicious input detected, risk_score=%.2f reason=%q", decision.RiskScore, decision.Reason)
	}

	if d := a.degradationCheck(ctx, "chat_stream"); d.Enabled {
		g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream phase=degraded reason=%s duration=%dms",
			id, requestID, d.Reason, time.Since(phaseStart).Milliseconds())
		_, filteredReason := filterOutput(ctx, "", []string{d.Reason})
		sink.SendMeta(ChatStreamMetaEvent{Mode: "degraded", Detail: filteredReason, Degraded: true, DegradationReason: d.Reason})
		sink.SendDetails(filteredReason)
		sink.SendText(d.Message)
		sink.SendEvent("done", "Stream completed")
		return &ChatStreamResult{Degraded: true, DegradationReason: d.Reason}, nil
	}

	mu := a.acquireSessionLock(id)
	defer a.releaseSessionLock(id, mu)

	sessionMem := memory.GetSimpleMemory(id)

	memorySvc := aiservice.NewMemoryService()
	ctxBuildStart := time.Now()
	contextPkg, contextDetail := memorySvc.BuildChatPackage(ctx, id, msg, sessionMem.GetContextMessages())
	g.Log().Debugf(ctx, "[session:%s][req:%s] ChatStream phase=context_built history=%d memory=%d docs=%d tools=%d duration=%dms",
		id, requestID, len(contextPkg.HistoryMessages), len(contextPkg.MemoryItems),
		len(contextPkg.DocumentItems), len(contextPkg.ToolItems), time.Since(ctxBuildStart).Milliseconds())

	userMessage := &chat_pipeline.UserMessage{
		ID:        id,
		Query:     msg,
		Documents: contextengine.DocumentsContent(contextPkg),
		History:   contextPkg.HistoryMessages,
	}

	sseBridge := events.NewStreamSinkEmitter(sink, requestID)
	traceEmitter := events.NewTraceEmitter(requestID)
	resultCollector := events.NewResultCollector()
	contractCollector := events.NewContractCollector()
	schemaGateCollector := events.NewSchemaGateCollector()
	multiEmitter := events.NewMultiEmitter(sseBridge, traceEmitter, events.GlobalHealthCollector(), resultCollector, contractCollector, schemaGateCollector)
	ctx = chat_pipeline.WithChatToolEmitter(ctx, multiEmitter, requestID)

	runner, agentBuildErr := a.buildChatAgent(ctx, msg)
	if agentBuildErr != nil {
		g.Log().Errorf(ctx, "[session:%s][req:%s] ChatStream phase=agent_build_failed error=%v duration=%dms",
			id, requestID, agentBuildErr, time.Since(phaseStart).Milliseconds())
		if status, message := classifyError(agentBuildErr); status != 0 {
			_, filteredDetail := filterOutput(ctx, "", []string{message})
			sink.SendMeta(ChatStreamMetaEvent{Mode: "degraded", Detail: filteredDetail, Degraded: true, DegradationReason: message})
			sink.SendDetails(filteredDetail)
			sink.SendText(message)
			sink.SendEvent("done", "Stream completed")
			return &ChatStreamResult{Degraded: true, DegradationReason: message}, nil
		}
		sink.SendEvent("error", agentBuildErr.Error())
		sink.SendEvent("done", "Stream completed")
		return &ChatStreamResult{}, nil
	}

	_, filteredDetail := filterOutput(ctx, "", contextDetail)
	g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream phase=agent_built duration=%dms",
		id, requestID, time.Since(phaseStart).Milliseconds())
	sink.SendMeta(ChatStreamMetaEvent{Mode: "chat", Detail: filteredDetail})
	sink.SendDetails(filteredDetail)

	callbackEmitter := events.NewModelCallbackEmitter(multiEmitter, requestID)
	sr, err := runner.Stream(ctx, userMessage, compose.WithCallbacks(
		log_call_back.LogCallback(nil),
		callbackEmitter.Handler(),
	))
	if err != nil {
		if status, message := classifyError(err); status != 0 {
			_, detailFiltered := filterOutput(ctx, "", []string{message})
			sink.SendMeta(ChatStreamMetaEvent{Mode: "degraded", Detail: detailFiltered, Degraded: true, DegradationReason: message})
			sink.SendDetails(detailFiltered)
			sink.SendText(message)
			sink.SendEvent("done", "Stream completed")
			return &ChatStreamResult{Degraded: true, DegradationReason: message}, nil
		}
		g.Log().Errorf(ctx, "[session:%s][req:%s] Agent stream failed: %v", id, requestID, err)
		sink.SendEvent("error", err.Error())
		sink.SendEvent("done", "Stream completed")
		return &ChatStreamResult{}, nil
	}
	defer sr.Close()

	var fullResponse strings.Builder

	hallucinationCfg, _ := events.HallucinationConfigFromYAML(ctx)

	defer func() {
		completeResponse := fullResponse.String()
		if completeResponse != "" {
			memorySvc.PersistOutcome(ctx, id, msg, completeResponse)
			g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream completed, answer length: %d, turns: %d",
				id, requestID, len(completeResponse), sessionMem.TurnCount())

			if hallucinationCfg == nil || !hallucinationCfg.Enabled {
				return
			}

			if hallucinationCfg.OutputValidationEnabled {
				if resultCollector.HasToolCalls() {
					toolResults := resultCollector.ToolResults()
					if warnings := events.ValidateOutputWithConfig(hallucinationCfg, completeResponse, toolResults); len(warnings) > 0 {
						g.Log().Warningf(ctx, "[session:%s][req:%s] output validation warnings: %v", id, requestID, warnings)
					}
				} else if hallucinationCfg.NoToolCallDetectionEnabled && isOpsRelatedQuery(msg, hallucinationCfg.OpsKeywords) {
					g.Log().Warningf(ctx, "[session:%s][req:%s] ops-related query but no tool calls detected, possible hallucination risk", id, requestID)
				}
			}

			if hallucinationCfg.ContractEnabled {
				contractResult := events.ValidateContractWithConfig(
					hallucinationCfg,
					completeResponse,
					contractCollector.ToolResults(),
					contractCollector.FailedTools(),
					contractCollector.HasToolCalls(),
				)
				if !contractResult.Passed {
					g.Log().Warningf(ctx, "[session:%s][req:%s] contract check: %s", id, requestID, contractResult.Summary())
					if hallucinationCfg.ContractBlockOnViolation {
						sink.SendEvent("contract_violation", contractResult.Summary())
					}
				} else if len(contractResult.Violations) > 0 {
					g.Log().Infof(ctx, "[session:%s][req:%s] contract check: %s", id, requestID, contractResult.Summary())
				}
			}

			if hallucinationCfg.SchemaGateEnabled {
				schemaGate := events.NewSchemaGateWithConfig(hallucinationCfg)
				schemaResult := schemaGate.Validate(completeResponse)
				if !schemaResult.Passed {
					g.Log().Warningf(ctx, "[session:%s][req:%s] schema gate: %s", id, requestID, schemaResult.Summary())
					sink.SendEvent("schema_gate", schemaResult.Summary())
				} else {
					for _, check := range schemaResult.Checks {
						if !check.Passed {
							g.Log().Infof(ctx, "[session:%s][req:%s] schema gate warn [%s]: %s", id, requestID, check.Field, check.Detail)
						}
					}
				}
			}

			if hallucinationCfg.LLMValidation != nil && hallucinationCfg.LLMValidation.Enabled && resultCollector.HasToolCalls() {
				toolResults := resultCollector.ToolResults()
				llmValidator := events.NewLLMValidator(models.OpenAIForGLMFast, hallucinationCfg.LLMValidation)
				llmResult := llmValidator.Validate(ctx, completeResponse, toolResults)
				if len(llmResult.OmissionWarnings) > 0 {
					g.Log().Warningf(ctx, "[session:%s][req:%s] LLM omission detection: %v", id, requestID, llmResult.OmissionWarnings)
				}
				if len(llmResult.AccuracyWarnings) > 0 {
					g.Log().Warningf(ctx, "[session:%s][req:%s] LLM accuracy check: %v", id, requestID, llmResult.AccuracyWarnings)
				}
			}
		}
	}()

	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			sink.SendEvent("done", "Stream completed")
			return &ChatStreamResult{FullResponse: fullResponse.String()}, nil
		}
		if err != nil {
			g.Log().Errorf(ctx, "[session:%s][req:%s] Stream recv error: %v", id, requestID, err)

			if fullResponse.Len() > 0 {
				g.Log().Infof(ctx, "[session:%s][req:%s] Stream interrupted but partial content (%d chars) already received, ending gracefully",
					id, requestID, fullResponse.Len())
				sink.SendEvent("done", "Stream completed")
				return &ChatStreamResult{FullResponse: fullResponse.String()}, nil
			}

			if status, message := classifyError(err); status != 0 {
				g.Log().Infof(ctx, "[session:%s][req:%s] ChatStream phase=stream_fallback reason=%s",
					id, requestID, message)
				_, filteredDetail := filterOutput(ctx, "", []string{message})
				sink.SendMeta(ChatStreamMetaEvent{Mode: "degraded", Detail: filteredDetail, Degraded: true, DegradationReason: message})
				sink.SendDetails(filteredDetail)
				sink.SendText(message)
				sink.SendEvent("done", "Stream completed")
				return &ChatStreamResult{Degraded: true, DegradationReason: message}, nil
			}

			sink.SendEvent("error", err.Error())
			sink.SendEvent("done", "Stream completed")
			return &ChatStreamResult{}, nil
		}
		filteredChunk, _ := filterOutput(ctx, chunk.Content, nil)
		fullResponse.WriteString(filteredChunk)
		sink.SendText(filteredChunk)
	}
}

// isOpsRelatedQuery checks if the query contains ops-related keywords.
func isOpsRelatedQuery(query string, keywords []string) bool {
	if len(keywords) == 0 {
		keywords = defaultOpsKeywords
	}
	lower := strings.ToLower(strings.TrimSpace(query))
	if lower == "" {
		return false
	}
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

var defaultOpsKeywords = []string{
	"告警", "alert", "prometheus", "日志", "log", "排查", "故障", "incident",
	"指标", "metric", "延迟", "latency", "错误率", "error rate", "超时", "timeout",
	"服务异常", "服务挂了", "报警", "cpu", "内存", "memory", "磁盘",
	"网络", "network", "数据库", "mysql", "redis", "连接池", "队列", "queue",
}
