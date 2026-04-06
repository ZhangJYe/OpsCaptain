package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/ai/tools"

	"github.com/gogf/gf/v2/frame/g"
)

const AgentName = "knowledge"

const defaultKnowledgeQueryTimeout = 5 * time.Second

var (
	newQueryInternalDocsTool = tools.NewQueryInternalDocsTool
	knowledgeQueryTimeout    = func(ctx context.Context) time.Duration {
		v, err := g.Cfg().Get(ctx, "multi_agent.knowledge_query_timeout_ms")
		if err == nil && v.Int64() > 0 {
			return time.Duration(v.Int64()) * time.Millisecond
		}
		return defaultKnowledgeQueryTimeout
	}
)

type Agent struct {
	registry *skills.Registry
}

type knowledgeSkill struct {
	name        string
	description string
	mode        string
	keywords    []string
	focus       string
}

func New() *Agent {
	return &Agent{registry: buildKnowledgeSkillRegistry()}
}

func (a *Agent) Name() string {
	return AgentName
}

func (a *Agent) Capabilities() []string {
	return skills.PrefixedCapabilities([]string{"knowledge-rag"}, a.registry.SkillNames())
}

func (a *Agent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	execution, err := a.registry.Execute(ctx, task)
	if err != nil {
		return nil, err
	}
	if rt, ok := runtime.FromContext(ctx); ok {
		rt.EmitInfo(ctx, task, a.Name(), fmt.Sprintf("selected skill=%s", execution.Skill.Name()), map[string]any{
			"skill_name":        execution.Skill.Name(),
			"skill_description": execution.Skill.Description(),
		})
	}
	return execution.Result, nil
}

func (s *knowledgeSkill) Name() string {
	return s.name
}

func (s *knowledgeSkill) Description() string {
	return s.description
}

func (s *knowledgeSkill) Match(task *protocol.TaskEnvelope) bool {
	if task == nil || len(s.keywords) == 0 {
		return len(s.keywords) == 0
	}
	return skills.ContainsAny(task.Goal, s.keywords...)
}

func (s *knowledgeSkill) Run(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	return runKnowledgeLookupWithFocus(ctx, task, s.mode, s.focus)
}

func buildKnowledgeSkillRegistry() *skills.Registry {
	registry, err := skills.NewRegistry(
		AgentName,
		&knowledgeSkill{
			name:        "knowledge_rollback_runbook",
			description: "Retrieve rollback, recovery, and mitigation runbooks for bad releases and incidents.",
			mode:        "rollback_runbook",
			focus:       "Focus on rollback triggers, mitigation actions, recovery steps, and validation checklist.",
			keywords: []string{
				"rollback", "revert", "recover", "recovery", "restore", "回滚", "恢复", "止损", "回退",
			},
		},
		&knowledgeSkill{
			name:        "knowledge_release_sop",
			description: "Retrieve release, deployment, and rollout SOPs with pre-check and rollback guidance.",
			mode:        "release_sop",
			focus:       "Focus on release, deployment, pre-check, post-check, verification, and rollback steps.",
			keywords: []string{
				"release", "deploy", "deployment", "rollout", "publish", "launch", "上线", "发版", "发布", "部署", "灰度",
			},
		},
		&knowledgeSkill{
			name:        "knowledge_sop_lookup",
			description: "Retrieve SOP, runbook, and internal documentation matches for explicit procedure questions.",
			mode:        "sop_lookup",
			focus:       "Focus on SOP, runbook, checklist, and operator step-by-step actions.",
			keywords: []string{
				"sop", "runbook", "playbook", "doc", "docs", "knowledge base",
				"文档", "知识库", "手册", "排障手册", "处理流程", "操作步骤", "SOP",
			},
		},
		&knowledgeSkill{
			name:        "knowledge_incident_guidance",
			description: "Fallback knowledge retrieval for broader incident analysis and troubleshooting guidance.",
			mode:        "incident_guidance",
			focus:       "Focus on troubleshooting guidance, mitigation steps, and related incident runbooks.",
		},
	)
	if err != nil {
		panic(fmt.Sprintf("failed to build knowledge skills registry: %v", err))
	}
	return registry
}

func runKnowledgeLookupWithFocus(ctx context.Context, task *protocol.TaskEnvelope, mode string, focus string) (*protocol.TaskResult, error) {
	tool := newQueryInternalDocsTool()
	retrievalQuery := buildKnowledgeQuery(task.Goal, focus)
	args, _ := json.Marshal(&tools.QueryInternalDocsInput{Query: retrievalQuery})
	queryCtx, cancel := context.WithTimeout(ctx, knowledgeQueryTimeout(ctx))
	defer cancel()

	output, err := tool.InvokableRun(queryCtx, string(args))
	if err != nil {
		summary := fmt.Sprintf("knowledge lookup failed: %v", err)
		if queryCtx.Err() == context.DeadlineExceeded {
			summary = "knowledge lookup timed out; skipped"
		}
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      AgentName,
			Status:     protocol.ResultStatusDegraded,
			Summary:    summary,
			Confidence: 0.25,
			Metadata: map[string]any{
				"error":           err.Error(),
				"knowledge_mode":  mode,
				"knowledge_query": retrievalQuery,
			},
		}, nil
	}

	var docs []map[string]any
	if err := json.Unmarshal([]byte(output), &docs); err != nil {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      AgentName,
			Status:     protocol.ResultStatusDegraded,
			Summary:    "knowledge lookup returned an unreadable document payload",
			Confidence: 0.3,
			Metadata: map[string]any{
				"raw_output":      output,
				"knowledge_mode":  mode,
				"knowledge_query": retrievalQuery,
				"decode_failed":   true,
				"document_count":  0,
			},
		}, nil
	}

	limit := knowledgeEvidenceLimit()
	evidence := make([]protocol.EvidenceItem, 0, min(limit, len(docs)))
	highlights := make([]string, 0, min(limit, len(docs)))
	for idx, doc := range docs {
		if idx >= limit {
			break
		}
		content := firstNonEmptyString(doc, "content", "page_content", "text")
		title := firstNonEmptyString(doc, "id", "title", "source")
		if title == "" {
			title = fmt.Sprintf("doc-%d", idx+1)
		}
		snippet := shorten(content, 160)
		if snippet != "" {
			highlights = append(highlights, snippet)
		}
		evidence = append(evidence, protocol.EvidenceItem{
			SourceType: "knowledge",
			SourceID:   title,
			Title:      title,
			Snippet:    snippet,
			Score:      0.74 - float64(idx)*0.08,
		})
	}

	summary := "knowledge lookup returned no reusable documents"
	if len(evidence) > 0 {
		summary = fmt.Sprintf("knowledge lookup found %d relevant documents", len(docs))
		if len(highlights) > 0 {
			summary += ": " + strings.Join(highlights, " | ")
		}
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      AgentName,
		Status:     protocol.ResultStatusSucceeded,
		Summary:    summary,
		Confidence: 0.78,
		Evidence:   evidence,
		Metadata: map[string]any{
			"document_count":  len(docs),
			"knowledge_mode":  mode,
			"knowledge_query": retrievalQuery,
		},
	}, nil
}

func buildKnowledgeQuery(goal string, focus string) string {
	goal = strings.TrimSpace(goal)
	focus = strings.TrimSpace(focus)
	if focus == "" {
		return goal
	}
	if goal == "" {
		return focus
	}
	return goal + "\nFocus: " + focus
}

func mustNewSkillRegistry() *skills.Registry {
	registry, err := skills.NewRegistry(
		AgentName,
		&knowledgeSkill{
			name:        "knowledge_sop_lookup",
			description: "Retrieve SOP, runbook, and internal documentation matches for explicit procedure questions.",
			mode:        "sop_lookup",
			keywords: []string{
				"sop", "runbook", "playbook", "doc", "docs", "knowledge base",
				"文档", "知识库", "手册", "排障手册", "处理流程", "操作步骤", "SOP",
			},
		},
		&knowledgeSkill{
			name:        "knowledge_incident_guidance",
			description: "Fallback knowledge retrieval for broader incident analysis and troubleshooting guidance.",
			mode:        "incident_guidance",
		},
	)
	if err != nil {
		panic(fmt.Sprintf("failed to build knowledge skills registry: %v", err))
	}
	return registry
}

func runKnowledgeLookup(ctx context.Context, task *protocol.TaskEnvelope, mode string) (*protocol.TaskResult, error) {
	tool := newQueryInternalDocsTool()
	args, _ := json.Marshal(&tools.QueryInternalDocsInput{Query: task.Goal})
	queryCtx, cancel := context.WithTimeout(ctx, knowledgeQueryTimeout(ctx))
	defer cancel()

	output, err := tool.InvokableRun(queryCtx, string(args))
	if err != nil {
		summary := fmt.Sprintf("knowledge lookup failed: %v", err)
		if queryCtx.Err() == context.DeadlineExceeded {
			summary = "knowledge lookup timed out; skipped"
		}
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      AgentName,
			Status:     protocol.ResultStatusDegraded,
			Summary:    summary,
			Confidence: 0.25,
			Metadata: map[string]any{
				"error":          err.Error(),
				"knowledge_mode": mode,
			},
		}, nil
	}

	var docs []map[string]any
	if err := json.Unmarshal([]byte(output), &docs); err != nil {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      AgentName,
			Status:     protocol.ResultStatusDegraded,
			Summary:    "knowledge lookup returned an unreadable document payload",
			Confidence: 0.3,
			Metadata: map[string]any{
				"raw_output":      output,
				"knowledge_mode":  mode,
				"decode_failed":   true,
				"document_count":  0,
				"decoded_payload": "invalid_json",
			},
		}, nil
	}

	limit := knowledgeEvidenceLimit()
	evidence := make([]protocol.EvidenceItem, 0, min(limit, len(docs)))
	highlights := make([]string, 0, min(limit, len(docs)))
	for idx, doc := range docs {
		if idx >= limit {
			break
		}
		content := firstNonEmptyString(doc, "content", "page_content", "text")
		title := firstNonEmptyString(doc, "id", "title", "source")
		if title == "" {
			title = fmt.Sprintf("doc-%d", idx+1)
		}
		snippet := shorten(content, 160)
		if snippet != "" {
			highlights = append(highlights, snippet)
		}
		evidence = append(evidence, protocol.EvidenceItem{
			SourceType: "knowledge",
			SourceID:   title,
			Title:      title,
			Snippet:    snippet,
			Score:      0.74 - float64(idx)*0.08,
		})
	}

	summary := "knowledge lookup returned no reusable documents"
	if len(evidence) > 0 {
		summary = fmt.Sprintf("knowledge lookup found %d relevant documents", len(docs))
		if len(highlights) > 0 {
			summary += ": " + strings.Join(highlights, " | ")
		}
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      AgentName,
		Status:     protocol.ResultStatusSucceeded,
		Summary:    summary,
		Confidence: 0.78,
		Evidence:   evidence,
		Metadata: map[string]any{
			"document_count": len(docs),
			"knowledge_mode": mode,
		},
	}, nil
}

func firstNonEmptyString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func shorten(input string, max int) string {
	input = strings.TrimSpace(strings.ReplaceAll(input, "\n", " "))
	if len(input) <= max {
		return input
	}
	return input[:max] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func knowledgeEvidenceLimit() int {
	v, err := g.Cfg().Get(context.Background(), "multi_agent.knowledge_evidence_limit")
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	v, err = g.Cfg().Get(context.Background(), "retriever.top_k")
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	return 3
}
