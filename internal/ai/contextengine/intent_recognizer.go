package contextengine

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const defaultIntentTimeout = 500 * time.Millisecond

type IntentType string

const (
	IntentFaultDiagnosis IntentType = "fault_diagnosis"
	IntentKnowledgeQuery IntentType = "knowledge_query"
	IntentChat           IntentType = "chat"
	IntentUnknown        IntentType = ""
)

type IntentResult struct {
	Type     IntentType
	Degraded bool
	LatencyMs int64
}

var intentProfileMap = map[IntentType]string{
	IntentFaultDiagnosis: "aiops_diagnosis",
	IntentKnowledgeQuery: "chat",
	IntentChat:           "chat",
	IntentUnknown:        "chat",
}

func ProfileForIntent(intent IntentType) string {
	if p, ok := intentProfileMap[intent]; ok {
		return p
	}
	return "chat"
}

type IntentRecognizer struct {
	modelFactory func(ctx context.Context) (model.ToolCallingChatModel, error)
	timeout      time.Duration
	modelOnce    sync.Once
	model        model.ToolCallingChatModel
	modelErr     error
}

func NewIntentRecognizer(modelFactory func(ctx context.Context) (model.ToolCallingChatModel, error)) *IntentRecognizer {
	return &IntentRecognizer{
		modelFactory: modelFactory,
		timeout:      defaultIntentTimeout,
	}
}

func (r *IntentRecognizer) ensureModel(ctx context.Context) error {
	r.modelOnce.Do(func() {
		r.model, r.modelErr = r.modelFactory(ctx)
	})
	return r.modelErr
}

func (r *IntentRecognizer) Recognize(ctx context.Context, query string) IntentResult {
	start := time.Now()

	if err := r.ensureModel(ctx); err != nil {
		return IntentResult{Type: IntentKnowledgeQuery, Degraded: true, LatencyMs: time.Since(start).Milliseconds()}
	}

	intentCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	prompt := buildIntentPrompt(query)
	resp, err := r.model.Generate(intentCtx, []*schema.Message{
		schema.UserMessage(prompt),
	})
	if err != nil {
		return IntentResult{Type: IntentKnowledgeQuery, Degraded: true, LatencyMs: time.Since(start).Milliseconds()}
	}

	intent, ok := parseIntent(resp.Content)
	if !ok {
		return IntentResult{Type: IntentKnowledgeQuery, Degraded: true, LatencyMs: time.Since(start).Milliseconds()}
	}

	return IntentResult{Type: intent, LatencyMs: time.Since(start).Milliseconds()}
}

func buildIntentPrompt(query string) string {
	return `判断以下问题的类型，只输出 JSON，不要添加其他文字：
{"type": "fault_diagnosis"}

可选类型：
- fault_diagnosis：故障排查（用户遇到了问题，需要诊断）
- knowledge_query：知识查询（用户想了解某个概念或配置）
- chat：闲聊（用户在闲聊或问候）

问题：` + query
}

func parseIntent(resp string) (IntentType, bool) {
	type intentResp struct {
		Type string `json:"type"`
	}
	var r intentResp
	if err := json.Unmarshal([]byte(resp), &r); err == nil {
		switch IntentType(r.Type) {
		case IntentFaultDiagnosis, IntentKnowledgeQuery, IntentChat:
			return IntentType(r.Type), true
		}
	}

	lower := strings.ToLower(resp)
	if strings.Contains(lower, "fault_diagnosis") || strings.Contains(lower, "故障") {
		return IntentFaultDiagnosis, true
	}
	if strings.Contains(lower, "knowledge_query") || strings.Contains(lower, "知识") {
		return IntentKnowledgeQuery, true
	}
	if strings.Contains(lower, "chat") || strings.Contains(lower, "闲聊") {
		return IntentChat, true
	}
	return IntentUnknown, false
}
