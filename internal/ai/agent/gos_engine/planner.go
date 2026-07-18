package gos_engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/promptreg"
)

type Planner struct {
	experts  map[string]experts.ExpertAgent
	cfg      *Config
	logger   Logger
	generate StructuredGenerateFunc
}

type PlanningHistory struct {
	CalledGoalKeys  map[string]struct{}
	FailedTools     map[string]struct{}
	RemainingBudget PlanBudgetConfig
}

type PlanningContext struct {
	Frontier        *belief.Frontier
	Graph           *belief.BeliefGraph
	CalledGoalKeys  map[string]struct{}
	FailedTools     map[string]struct{}
	RemainingBudget PlanBudgetConfig
}

type PlanOutcome struct {
	Items          []PlanItem
	Mode           string
	FallbackReason string
	FallbackDetail string
	LLMCalls       int
}

type planProposal struct {
	Items []PlanItem `json:"items"`
}

func NewPlanner(experts map[string]experts.ExpertAgent, cfg *Config, logger Logger) *Planner {
	return NewStructuredPlanner(experts, cfg, logger, nil)
}

func NewStructuredPlanner(experts map[string]experts.ExpertAgent, cfg *Config, logger Logger, generate StructuredGenerateFunc) *Planner {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Planner{experts: experts, cfg: cfg, logger: logger, generate: generate}
}

func NewPlanningHistory() *PlanningHistory {
	return &PlanningHistory{
		CalledGoalKeys: make(map[string]struct{}),
		FailedTools:    make(map[string]struct{}),
	}
}

func (p *Planner) Plan(ctx context.Context, frontier *belief.Frontier) ([]PlanItem, error) {
	outcome, err := p.PlanWithContext(ctx, PlanningContext{
		Frontier:        frontier,
		CalledGoalKeys:  map[string]struct{}{},
		FailedTools:     map[string]struct{}{},
		RemainingBudget: scalePlanBudget(p.cfg.StructuredCognition.PlanBudget, p.cfg.StructuredCognition.MaxPlanItems),
	})
	return outcome.Items, err
}

func (p *Planner) PlanWithContext(ctx context.Context, planning PlanningContext) (PlanOutcome, error) {
	if err := ctx.Err(); err != nil {
		return PlanOutcome{}, err
	}
	if len(p.experts) == 0 {
		if p.logger != nil {
			p.logger.Info("plan generated", "expert_count", 0)
		}
		return PlanOutcome{}, nil
	}
	if planning.Frontier == nil || strings.TrimSpace(planning.Frontier.NodeID) == "" {
		return PlanOutcome{}, fmt.Errorf("frontier is required")
	}
	if planning.CalledGoalKeys == nil {
		planning.CalledGoalKeys = map[string]struct{}{}
	}
	if planning.FailedTools == nil {
		planning.FailedTools = map[string]struct{}{}
	}
	if planning.RemainingBudget == (PlanBudgetConfig{}) {
		planning.RemainingBudget = scalePlanBudget(p.cfg.StructuredCognition.PlanBudget, p.cfg.StructuredCognition.MaxPlanItems)
	}

	outcome := PlanOutcome{Mode: "rules"}
	if p.cfg.StructuredCognition.Enabled {
		outcome.Mode = "rule_fallback"
		if p.generate == nil {
			outcome.FallbackReason = "structured_generator_unavailable"
		} else {
			prompt, err := p.buildStructuredPrompt(planning)
			if err != nil {
				return PlanOutcome{}, err
			}
			outcome.LLMCalls = 1
			raw, err := p.generate(ctx, prompt)
			if ctx.Err() != nil {
				return outcome, ctx.Err()
			}
			if err != nil {
				outcome.FallbackReason = "structured_generation_failed"
				outcome.FallbackDetail = err.Error()
			} else {
				var proposal planProposal
				if err := decodeStrictJSONObject(raw, &proposal); err != nil {
					outcome.FallbackReason = "structured_schema_invalid"
					outcome.FallbackDetail = err.Error()
				} else {
					proposal.Items = p.normalizePlanBudgets(proposal.Items)
					if err := p.validatePlan(proposal.Items, planning); err != nil {
						outcome.FallbackReason = "structured_contract_invalid"
						outcome.FallbackDetail = err.Error()
					} else {
						for index := range proposal.Items {
							proposal.Items[index].TargetHypothesisID = planning.Frontier.NodeID
						}
						outcome.Items = proposal.Items
						outcome.Mode = "structured"
					}
				}
			}
		}
	}

	if len(outcome.Items) == 0 {
		item, err := p.fallbackPlan(planning, outcome.FallbackReason)
		if err != nil {
			return outcome, err
		}
		outcome.Items = []PlanItem{item}
	}
	if p.logger != nil {
		p.logger.Info("plan generated",
			"expert_count", len(outcome.Items),
			"mode", outcome.Mode,
			"fallback_reason", outcome.FallbackReason,
			"fallback_detail", outcome.FallbackDetail,
		)
	}
	return outcome, nil
}

func (p *Planner) buildStructuredPrompt(planning PlanningContext) (string, error) {
	contextValue := struct {
		Frontier        *belief.Frontier         `json:"frontier"`
		ActiveGraph     []map[string]interface{} `json:"active_graph"`
		MissingEvidence []string                 `json:"missing_evidence"`
		CalledGoalKeys  []string                 `json:"called_goal_keys"`
		FailedTools     []string                 `json:"failed_tools"`
		Experts         []plannerExpertContext   `json:"experts"`
		RemainingBudget PlanBudgetConfig         `json:"remaining_budget"`
		MaxPlanItems    int                      `json:"max_plan_items"`
	}{
		Frontier:        planning.Frontier,
		ActiveGraph:     activeGraphSummary(planning.Graph),
		MissingEvidence: inferMissingEvidence(planning.Frontier),
		CalledGoalKeys:  sortedSetKeys(planning.CalledGoalKeys),
		FailedTools:     sortedSetKeys(planning.FailedTools),
		Experts:         p.plannerExperts(),
		RemainingBudget: planning.RemainingBudget,
		MaxPlanItems:    p.cfg.StructuredCognition.MaxPlanItems,
	}
	raw, err := json.Marshal(contextValue)
	if err != nil {
		return "", fmt.Errorf("marshal planning context: %w", err)
	}
	return renderStructuredPrompt(promptreg.GOSPlanner, map[string]string{"planning_context": string(raw)}), nil
}

type plannerExpertContext struct {
	Name   string           `json:"name"`
	Tools  []string         `json:"tools"`
	Budget PlanBudgetConfig `json:"budget"`
}

func (p *Planner) plannerExperts() []plannerExpertContext {
	names := make([]string, 0, len(p.experts))
	for name := range p.experts {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]plannerExpertContext, 0, len(names))
	for _, name := range names {
		out = append(out, plannerExpertContext{Name: name, Tools: p.expertTools(name), Budget: p.expertBudget(name)})
	}
	return out
}

func (p *Planner) normalizePlanBudgets(items []PlanItem) []PlanItem {
	for index := range items {
		if _, exists := p.experts[items[index].ExpertName]; !exists {
			continue
		}
		items[index].Budget = normalizePlanBudget(
			items[index].Budget,
			p.cfg.StructuredCognition.PlanBudget,
			p.expertBudget(items[index].ExpertName),
		)
	}
	return items
}

func normalizePlanBudget(requested, fallback, limit PlanBudgetConfig) PlanBudgetConfig {
	return PlanBudgetConfig{
		LLMCalls:          normalizeBudgetValue(requested.LLMCalls, fallback.LLMCalls, limit.LLMCalls),
		ToolCalls:         normalizeBudgetValue(requested.ToolCalls, fallback.ToolCalls, limit.ToolCalls),
		RAGCalls:          normalizeBudgetValue(requested.RAGCalls, fallback.RAGCalls, limit.RAGCalls),
		TimeoutMs:         normalizeBudgetValue(requested.TimeoutMs, fallback.TimeoutMs, limit.TimeoutMs),
		MaxRetrievalSteps: normalizeBudgetValue(requested.MaxRetrievalSteps, fallback.MaxRetrievalSteps, limit.MaxRetrievalSteps),
		MaxOutputTokens:   normalizeBudgetValue(requested.MaxOutputTokens, fallback.MaxOutputTokens, limit.MaxOutputTokens),
	}
}

func normalizeBudgetValue(requested, fallback, limit int) int {
	if requested <= 0 {
		requested = fallback
	}
	if requested <= 0 || requested > limit {
		requested = limit
	}
	return requested
}

func (p *Planner) validatePlan(items []PlanItem, planning PlanningContext) error {
	maxItems := p.cfg.StructuredCognition.MaxPlanItems
	if maxItems <= 0 || len(items) == 0 || len(items) > maxItems {
		return fmt.Errorf("plan must contain 1..%d items", maxItems)
	}
	remaining := planning.RemainingBudget
	used := PlanBudgetConfig{}
	seenGoals := make(map[string]struct{}, len(items))
	for index, item := range items {
		if _, ok := p.experts[item.ExpertName]; !ok {
			return fmt.Errorf("plan item %d references unknown expert %q", index, item.ExpertName)
		}
		if strings.TrimSpace(item.Reason) == "" || len(nonEmptyStrings(item.ExpectedEvidence)) == 0 || len(nonEmptyStrings(item.AllowedTools)) == 0 || len(nonEmptyStrings(item.StopConditions)) == 0 {
			return fmt.Errorf("plan item %d requires reason, expected evidence, allowed tools and stop conditions", index)
		}
		if !positivePlanBudget(item.Budget) {
			return fmt.Errorf("plan item %d budget must be positive", index)
		}
		if exceedsPlanBudget(item.Budget, p.expertBudget(item.ExpertName)) {
			return fmt.Errorf("plan item %d exceeds expert %q budget", index, item.ExpertName)
		}
		authorized := stringSet(p.expertTools(item.ExpertName))
		for _, toolName := range item.AllowedTools {
			if _, ok := authorized[toolName]; !ok {
				return fmt.Errorf("plan item %d tool %q is not authorized", index, toolName)
			}
			if _, failed := planning.FailedTools[toolName]; failed {
				return fmt.Errorf("plan item %d reuses failed tool %q", index, toolName)
			}
		}
		item.TargetHypothesisID = planning.Frontier.NodeID
		goalKey := planGoalKey(item)
		if _, exists := planning.CalledGoalKeys[goalKey]; exists {
			return fmt.Errorf("plan item %d repeats an existing evidence goal", index)
		}
		if _, exists := seenGoals[goalKey]; exists {
			return fmt.Errorf("plan contains duplicate evidence goal")
		}
		seenGoals[goalKey] = struct{}{}
		used = addPlanBudgets(used, item.Budget)
	}
	if exceedsPlanBudget(used, remaining) {
		return fmt.Errorf("plan exceeds remaining budget")
	}
	return nil
}

func (p *Planner) fallbackPlan(planning PlanningContext, fallbackReason string) (PlanItem, error) {
	preferred := p.pickExpert(planning.Frontier)
	names := make([]string, 0, len(p.experts))
	if _, exists := p.experts[preferred]; exists {
		names = append(names, preferred)
	}
	for _, name := range p.sortedExpertNames() {
		if name != preferred {
			names = append(names, name)
		}
	}
	for _, name := range names {
		tools := filterFailedTools(p.expertTools(name), planning.FailedTools)
		if len(tools) == 0 {
			continue
		}
		item := PlanItem{
			ExpertName:         name,
			TargetHypothesisID: planning.Frontier.NodeID,
			Reason:             "deterministic_frontier_match",
			ExpectedEvidence:   expectedEvidence(planning.Frontier, name),
			AllowedTools:       tools,
			StopConditions:     []string{"获得可验证的支持或反驳证据", "授权工具失败或预算耗尽"},
			Budget:             p.expertBudget(name),
			FallbackReason:     fallbackReason,
		}
		if exceedsPlanBudget(item.Budget, planning.RemainingBudget) {
			continue
		}
		if _, called := planning.CalledGoalKeys[planGoalKey(item)]; called {
			continue
		}
		return item, nil
	}
	return PlanItem{}, fmt.Errorf("no expert has a new evidence goal with an available authorized tool")
}

func (p *Planner) pickExpert(frontier *belief.Frontier) string {
	text := ""
	if frontier != nil {
		text = strings.ToLower(frontier.Label + " " + frontier.Why)
	}
	switch {
	case strings.Contains(text, "网络") || strings.Contains(text, "network") || strings.Contains(text, "latency") || strings.Contains(text, "timeout"):
		return "network_sre"
	case strings.Contains(text, "数据库") || strings.Contains(text, "mysql") || strings.Contains(text, "redis") || strings.Contains(text, "cache") || strings.Contains(text, "kafka") || strings.Contains(text, "tidb") || strings.Contains(text, "tikv"):
		return "database_sre"
	default:
		return "linux_sre"
	}
}

func (p *Planner) sortedExpertNames() []string {
	names := make([]string, 0, len(p.experts))
	for name := range p.experts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p *Planner) expertTools(name string) []string {
	for _, expert := range p.cfg.Experts {
		if expert.Name == name {
			return append([]string(nil), expert.Tools...)
		}
	}
	return []string{"query_logs", "query_internal_docs"}
}

func (p *Planner) expertBudget(name string) PlanBudgetConfig {
	for _, expert := range p.cfg.Experts {
		if expert.Name == name && positivePlanBudget(expert.Budget) {
			return expert.Budget
		}
	}
	return p.cfg.StructuredCognition.PlanBudget
}

func activeGraphSummary(graph *belief.BeliefGraph) []map[string]interface{} {
	if graph == nil {
		return nil
	}
	nodes := graph.GetActiveNodeCopies()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	out := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, map[string]interface{}{
			"id": node.ID, "type": node.Type, "label": node.Label, "score": node.Score, "level": node.Level,
		})
	}
	return out
}

func inferMissingEvidence(frontier *belief.Frontier) []string {
	if frontier == nil {
		return []string{"frontier evidence"}
	}
	missing := expectedEvidence(frontier, "")
	if frontier.Supports == 0 {
		missing = append(missing, "至少一条 source-backed support evidence")
	}
	if frontier.Refutes == 0 {
		missing = append(missing, "可能反驳当前假设的冲突证据")
	}
	return nonEmptyStrings(missing)
}

func expectedEvidence(frontier *belief.Frontier, expertName string) []string {
	text := strings.ToLower(expertName)
	if frontier != nil {
		text += " " + strings.ToLower(frontier.Label+" "+frontier.Why)
	}
	switch {
	case strings.Contains(text, "network") || strings.Contains(text, "网络") || strings.Contains(text, "latency") || strings.Contains(text, "timeout"):
		return []string{"延迟、丢包、DNS 或调用链传播证据"}
	case strings.Contains(text, "database") || strings.Contains(text, "数据库") || strings.Contains(text, "mysql") || strings.Contains(text, "redis") || strings.Contains(text, "tidb") || strings.Contains(text, "tikv"):
		return []string{"数据库连接、慢查询、存储或缓存状态证据"}
	default:
		return []string{"CPU、内存、磁盘、Pod 状态或应用错误证据"}
	}
}

func planGoalKey(item PlanItem) string {
	evidence := nonEmptyStrings(item.ExpectedEvidence)
	sort.Strings(evidence)
	return strings.ToLower(strings.TrimSpace(item.TargetHypothesisID) + "\x1f" + strings.TrimSpace(item.ExpertName) + "\x1f" + strings.Join(evidence, "\x1e"))
}

func sortedSetKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func filterFailedTools(tools []string, failed map[string]struct{}) []string {
	out := make([]string, 0, len(tools))
	for _, toolName := range tools {
		if _, exists := failed[toolName]; !exists {
			out = append(out, toolName)
		}
	}
	return out
}

func positivePlanBudget(budget PlanBudgetConfig) bool {
	return budget.LLMCalls > 0 && budget.ToolCalls > 0 && budget.RAGCalls > 0 && budget.TimeoutMs > 0 && budget.MaxRetrievalSteps > 0 && budget.MaxOutputTokens > 0
}

func addPlanBudgets(left, right PlanBudgetConfig) PlanBudgetConfig {
	return PlanBudgetConfig{
		LLMCalls:          left.LLMCalls + right.LLMCalls,
		ToolCalls:         left.ToolCalls + right.ToolCalls,
		RAGCalls:          left.RAGCalls + right.RAGCalls,
		TimeoutMs:         left.TimeoutMs + right.TimeoutMs,
		MaxRetrievalSteps: left.MaxRetrievalSteps + right.MaxRetrievalSteps,
		MaxOutputTokens:   left.MaxOutputTokens + right.MaxOutputTokens,
	}
}

func exceedsPlanBudget(used, remaining PlanBudgetConfig) bool {
	return used.LLMCalls > remaining.LLMCalls || used.ToolCalls > remaining.ToolCalls || used.RAGCalls > remaining.RAGCalls || used.TimeoutMs > remaining.TimeoutMs || used.MaxRetrievalSteps > remaining.MaxRetrievalSteps || used.MaxOutputTokens > remaining.MaxOutputTokens
}

func scalePlanBudget(budget PlanBudgetConfig, factor int) PlanBudgetConfig {
	if factor <= 0 {
		factor = 1
	}
	return PlanBudgetConfig{
		LLMCalls:          budget.LLMCalls * factor,
		ToolCalls:         budget.ToolCalls * factor,
		RAGCalls:          budget.RAGCalls * factor,
		TimeoutMs:         budget.TimeoutMs * factor,
		MaxRetrievalSteps: budget.MaxRetrievalSteps * factor,
		MaxOutputTokens:   budget.MaxOutputTokens * factor,
	}
}

func subtractPlanBudget(remaining, used PlanBudgetConfig) (PlanBudgetConfig, error) {
	if exceedsPlanBudget(used, remaining) {
		return PlanBudgetConfig{}, fmt.Errorf("plan exceeds remaining session budget")
	}
	return PlanBudgetConfig{
		LLMCalls:          remaining.LLMCalls - used.LLMCalls,
		ToolCalls:         remaining.ToolCalls - used.ToolCalls,
		RAGCalls:          remaining.RAGCalls - used.RAGCalls,
		TimeoutMs:         remaining.TimeoutMs - used.TimeoutMs,
		MaxRetrievalSteps: remaining.MaxRetrievalSteps - used.MaxRetrievalSteps,
		MaxOutputTokens:   remaining.MaxOutputTokens - used.MaxOutputTokens,
	}, nil
}
