package contracts

import (
	"fmt"
	"sort"
	"strings"

	"SuperBizAgent/internal/ai/protocol"
)

const (
	CacheScopeGlobal = "global"
	Version          = "2026-04-21"
)

type Contract struct {
	Agent            string
	Version          string
	CacheScope       string
	Role             string
	Responsibilities []string
	Inputs           []string
	Outputs          []string
	Must             []string
	MustNot          []string
	EvidencePolicy   []string
}

func (c Contract) ID() string {
	if c.Version == "" {
		return c.Agent
	}
	return c.Agent + ":" + c.Version
}

var registry = map[string]Contract{
	"metrics": {
		Agent:      "metrics",
		Version:    Version,
		CacheScope: CacheScopeGlobal,
		Role:       "指标 specialist，负责发现 Prometheus 指标、查询告警、当前指标值、指标趋势和指标相关健康信号。",
		Responsibilities: []string{
			"按任务选择指标技能，例如发布守卫、容量快照、告警分诊。",
			"把 Prometheus 指标发现、告警、即时指标和区间指标返回内容整理为 EvidenceItem。",
			"失败时返回 degraded，而不是中断 supervisor 编排。",
		},
		Inputs: []string{
			"specialist query",
			"Prometheus metric discovery result",
			"Prometheus alert query result",
			"Prometheus instant query result",
			"Prometheus range query result",
			"skill focus",
		},
		Outputs: []string{
			"metrics summary",
			"prometheus evidence",
			"next actions",
		},
		Must: []string{
			"区分 no active alerts、query failed、payload unreadable。",
			"保留 metric name、label keys、alert name、description、series labels、current value、trend summary 和 mode/focus metadata。",
			"需要发布判断时提示对比发布时间窗和回滚条件。",
		},
		MustNot: []string{
			"不要把指标告警推断成日志证据。",
			"不要在没有 Prometheus 结果时给出强根因。",
			"不要吞掉查询失败或超时。",
		},
		EvidencePolicy: []string{
			"Prometheus metric discovery、active alert、instant query value 和 range query trend 是实时指标证据。",
			"指标证据只能支持现象、范围和风险判断，根因需要结合 logs/knowledge。",
		},
	},
	"logs": {
		Agent:      "logs",
		Version:    Version,
		CacheScope: CacheScopeGlobal,
		Role:       "日志 specialist，负责通过 MCP 日志工具抽取错误、超时、panic、依赖失败等证据。",
		Responsibilities: []string{
			"按任务选择日志技能，例如 panic trace、API failure、payment timeout。",
			"把结构化日志或可复用 raw snippet 转为 EvidenceItem。",
			"保留工具降级原因，支持 reporter 透明汇总。",
		},
		Inputs: []string{
			"specialist query",
			"log MCP tools",
			"skill focus",
		},
		Outputs: []string{
			"log summary",
			"log evidence",
			"tool errors",
		},
		Must: []string{
			"区分结构化日志证据和 raw log fallback。",
			"保留 successful_tool、tool_errors、log_mode、log_focus metadata。",
			"日志工具不可用时返回 degraded。",
		},
		MustNot: []string{
			"不要把历史复盘标签当实时日志证据。",
			"不要把 raw output 伪装成已结构化验证的结论。",
			"不要因为单个日志工具失败就终止全部日志排查。",
		},
		EvidencePolicy: []string{
			"日志 evidence 必须包含来源工具、标题和片段。",
			"raw log fallback 只能作为弱证据，需提示后续验证。",
		},
	},
	"knowledge": {
		Agent:      "knowledge",
		Version:    Version,
		CacheScope: CacheScopeGlobal,
		Role:       "知识库 specialist，负责检索 SOP、runbook、错误码解释和历史处理经验。",
		Responsibilities: []string{
			"按任务选择知识技能，例如 release SOP、rollback runbook、error code lookup。",
			"把检索文档摘要为 knowledge EvidenceItem。",
			"失败时返回 degraded 并保留 retrieval query。",
		},
		Inputs: []string{
			"specialist query",
			"internal docs retriever",
			"skill focus",
		},
		Outputs: []string{
			"knowledge summary",
			"document evidence",
			"retrieval metadata",
		},
		Must: []string{
			"区分 SOP、runbook、历史复盘和实时证据。",
			"保留 document_count、knowledge_mode、knowledge_query metadata。",
			"错误码任务要提取 error code 并提示确认来源服务。",
		},
		MustNot: []string{
			"不要把历史标签当实时证据。",
			"不要把知识库建议包装成已发生事实。",
			"不要在无文档命中时编造 SOP 内容。",
		},
		EvidencePolicy: []string{
			"知识库 evidence 是指导和背景，不等价于实时观测。",
			"涉及根因时必须和 metrics/logs 或用户提供事实交叉验证。",
		},
	},
}

func Get(agent string) (Contract, bool) {
	contract, ok := registry[strings.ToLower(strings.TrimSpace(agent))]
	return contract, ok
}

func All() []Contract {
	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]Contract, 0, len(keys))
	for _, key := range keys {
		out = append(out, registry[key])
	}
	return out
}

func Capability(agent string) string {
	contract, ok := Get(agent)
	if !ok {
		return "contract:unknown"
	}
	return "contract:" + contract.ID()
}

func PromptFor(agent string) (string, bool) {
	contract, ok := Get(agent)
	if !ok {
		return "", false
	}
	return Render(contract), true
}

func Render(contract Contract) string {
	sections := []string{
		fmt.Sprintf("<!-- scope: %s -->", contract.CacheScope),
		fmt.Sprintf("# Agent Contract: %s", contract.Agent),
		fmt.Sprintf("Version: %s", contract.Version),
		"## Role\n" + contract.Role,
		renderList("Responsibilities", contract.Responsibilities),
		renderList("Inputs", contract.Inputs),
		renderList("Outputs", contract.Outputs),
		renderList("Must", contract.Must),
		renderList("Must Not", contract.MustNot),
		renderList("Evidence Policy", contract.EvidencePolicy),
	}
	return strings.Join(nonEmpty(sections), "\n\n")
}

func AttachMetadata(result *protocol.TaskResult, agent string) *protocol.TaskResult {
	if result == nil {
		return nil
	}
	contract, ok := Get(agent)
	if !ok {
		return result
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]any, 4)
	}
	result.Metadata["agent_contract_id"] = contract.ID()
	result.Metadata["agent_contract_scope"] = contract.CacheScope
	result.Metadata["agent_contract_version"] = contract.Version
	result.Metadata["agent_contract_role"] = contract.Role
	return result
}

func renderList(title string, values []string) string {
	if len(values) == 0 {
		return ""
	}
	lines := make([]string, 0, len(values)+1)
	lines = append(lines, "## "+title)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		lines = append(lines, "- "+value)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
