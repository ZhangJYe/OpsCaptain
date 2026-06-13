package knowledge

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	agentcontracts "SuperBizAgent/internal/ai/agent/contracts"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/ai/tools"

	"github.com/gogf/gf/v2/frame/g"
)

const AgentName = "knowledge"

const defaultKnowledgeQueryTimeout = 5 * time.Second

const (
	confidenceKnowledgeSucceeded = 0.78
	confidenceKnowledgeEvidence  = 0.74
	confidenceDocumentUnreadable = 0.30
	confidenceLookupFailed       = 0.25
)

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
	matcher     func(*protocol.TaskEnvelope) bool
}

func New() *Agent {
	return &Agent{registry: buildKnowledgeSkillRegistry()}
}

func SkillRegistry() *skills.Registry {
	return buildKnowledgeSkillRegistry()
}

func (a *Agent) Name() string {
	return AgentName
}

func (a *Agent) Capabilities() []string {
	return skills.PrefixedCapabilities([]string{"knowledge-rag", agentcontracts.Capability(AgentName)}, a.registry.SkillNames())
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
	return agentcontracts.AttachMetadata(execution.Result, AgentName), nil
}

func (s *knowledgeSkill) Name() string {
	return s.name
}

func (s *knowledgeSkill) Description() string {
	return s.description
}

func (s *knowledgeSkill) Match(task *protocol.TaskEnvelope) bool {
	if task == nil {
		return false
	}
	if s.matcher != nil {
		return s.matcher(task)
	}
	if len(s.keywords) == 0 {
		return false
	}
	return skills.ContainsAny(task.Goal, s.keywords...)
}

func (s *knowledgeSkill) Run(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	return runKnowledgeLookupWithFocus(ctx, task, s.mode, s.focus)
}

func (s *knowledgeSkill) Focus() string {
	return s.focus
}

func buildKnowledgeSkillRegistry() *skills.Registry {
	registry, err := skills.NewRegistry(
		AgentName,
		[]skills.Skill{
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
				name:        "knowledge_service_error_code_lookup",
				description: "Retrieve service error code explanations, common causes, and operator checks.",
				mode:        "service_error_code_lookup",
				focus:       "Focus on exact error code meaning, common causes, affected dependency, and first troubleshooting checks.",
				keywords:    []string{"error code", "errno", "status code", "code meaning"},
				matcher:     matchesServiceErrorCodeTask,
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
		},
		skills.WithDefault("knowledge_incident_guidance"),
	)
	if err != nil {
		g.Log().Errorf(context.Background(), "failed to build knowledge skills registry: %v", err)
		return nil
	}
	return registry
}

var serviceErrorCodePattern = regexp.MustCompile(`\b\d{6,}\b`)

func matchesServiceErrorCodeTask(task *protocol.TaskEnvelope) bool {
	if task == nil {
		return false
	}
	goal := strings.TrimSpace(task.Goal)
	if goal == "" {
		return false
	}
	if len(extractErrorCodes(goal)) == 0 {
		return false
	}
	return skills.ContainsAny(goal, "error code", "errno", "status code", "错误码", "错误代码", "返回码", "code")
}

func extractErrorCodes(goal string) []string {
	return serviceErrorCodePattern.FindAllString(goal, -1)
}