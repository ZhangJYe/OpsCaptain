package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/agent/gos_engine"
	"SuperBizAgent/internal/ai/agent/plan_execute_replan"
	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
	aitools "SuperBizAgent/internal/ai/tools"

	"github.com/cloudwego/eino/components/tool"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	aiOpsPlanAgentName = "aiops_plan_execute_replan"
	aiOpsGOSAgentName  = "aiops_gos_engine"
)

var (
	aiOpsRuntimeMu        sync.Mutex
	aiOpsRuntimes         = make(map[string]*runtime.Runtime)
	registerAIOpsAgentsFn = registerAIOpsAgents
	buildPlanAgent        = plan_execute_replan.BuildPlanAgent
	buildAIOpsGoSEngine   = newAIOpsGoSEngine
	newPersistentRuntime  = runtime.NewPersistent
	aiOpsConfigString     = func(ctx context.Context, key string) (string, bool) {
		v, err := g.Cfg().Get(ctx, key)
		if err != nil || strings.TrimSpace(v.String()) == "" {
			return "", false
		}
		return strings.TrimSpace(v.String()), true
	}
	aiOpsConfigBool = func(ctx context.Context, key string) (bool, bool) {
		v, err := g.Cfg().Get(ctx, key)
		if err != nil || strings.TrimSpace(v.String()) == "" {
			return false, false
		}
		return v.Bool(), true
	}
	aiOpsConfigInt = func(ctx context.Context, key string) (int, bool) {
		v, err := g.Cfg().Get(ctx, key)
		if err != nil || v.Int() <= 0 {
			return 0, false
		}
		return v.Int(), true
	}
	aiOpsConfigFloat = func(ctx context.Context, key string) (float64, bool) {
		v, err := g.Cfg().Get(ctx, key)
		if err != nil || strings.TrimSpace(v.String()) == "" {
			return 0, false
		}
		return v.Float64(), true
	}
)

type aiOpsEngineContextKey struct{}

type aiOpsPlanAgent struct{}

func (a *aiOpsPlanAgent) Name() string {
	return aiOpsPlanAgentName
}

func (a *aiOpsPlanAgent) Capabilities() []string {
	return []string{"ai_ops_analysis", "plan_execute_replan"}
}

func (a *aiOpsPlanAgent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	query := ""
	if task != nil {
		query = strings.TrimSpace(task.Goal)
	}
	if task != nil && task.Input != nil {
		if raw, ok := task.Input["executable_query"].(string); ok && strings.TrimSpace(raw) != "" {
			query = strings.TrimSpace(raw)
		}
	}

	if rt, ok := runtime.FromContext(ctx); ok && task != nil {
		ctx = plan_execute_replan.WithStageEmitter(ctx, func(emitCtx context.Context, message string, payload map[string]any) {
			rt.EmitInfo(emitCtx, task, a.Name(), message, payload)
		})
	}

	content, planDetail, err := buildPlanAgent(ctx, query)
	if rt, ok := runtime.FromContext(ctx); ok && task != nil {
		for _, step := range planDetail {
			step = strings.TrimSpace(step)
			if step == "" {
				continue
			}
			rt.EmitInfo(ctx, task, a.Name(), step, map[string]any{"plan_detail": true})
		}
	}
	if err != nil {
		summary := strings.TrimSpace(content)
		if summary == "" {
			summary = fmt.Sprintf("Plan 执行没有生成可靠结论：%v。请补充服务名、告警时间窗、关键日志或指标后重试。", err)
		}
		return &protocol.TaskResult{
			TaskID:            task.TaskID,
			Agent:             a.Name(),
			Status:            protocol.ResultStatusDegraded,
			Summary:           summary,
			Confidence:        0.15,
			DegradationReason: err.Error(),
			Evidence:          planEvidenceFromDetails(planDetail),
			Error: &protocol.TaskError{
				Code:    "plan_execute_replan_failed",
				Message: err.Error(),
			},
		}, nil
	}
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    strings.TrimSpace(content),
		Confidence: 0.8,
		Evidence:   planEvidenceFromDetails(planDetail),
	}, nil
}

func planEvidenceFromDetails(detail []string) []protocol.EvidenceItem {
	const maxSnippetRunes = 4000

	pendingTools := make([]string, 0)
	seen := make(map[string]struct{})
	evidence := make([]protocol.EvidenceItem, 0)
	for _, item := range detail {
		pendingTools = append(pendingTools, planToolNames(item)...)

		raw, ok := strings.CutPrefix(strings.TrimSpace(item), "tool:")
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		raw = strings.TrimSpace(raw)
		toolName := "plan_tool_result"
		if len(pendingTools) > 0 {
			toolName = pendingTools[0]
			pendingTools = pendingTools[1:]
		}
		sum := sha256.Sum256([]byte(raw))
		sourceID := fmt.Sprintf("%s:%x", toolName, sum[:8])
		if _, exists := seen[sourceID]; exists {
			continue
		}
		seen[sourceID] = struct{}{}

		runes := []rune(raw)
		if len(runes) > maxSnippetRunes {
			raw = string(runes[:maxSnippetRunes]) + "..."
		}
		evidence = append(evidence, protocol.EvidenceItem{
			SourceType: "tool",
			SourceID:   sourceID,
			Title:      toolName + " 工具结果",
			Snippet:    raw,
			Score:      1,
		})
	}
	return evidence
}

func planToolNames(detail string) []string {
	const marker = "Function:{Name:"

	names := make([]string, 0)
	remaining := detail
	for {
		index := strings.Index(remaining, marker)
		if index < 0 {
			return names
		}
		remaining = remaining[index+len(marker):]
		end := strings.IndexAny(remaining, " }\t\r\n")
		if end < 0 {
			end = len(remaining)
		}
		if name := strings.TrimSpace(remaining[:end]); name != "" {
			names = append(names, name)
		}
		remaining = remaining[end:]
	}
}

type aiOpsGOSAgent struct{}

func (a *aiOpsGOSAgent) Name() string {
	return aiOpsGOSAgentName
}

func (a *aiOpsGOSAgent) Capabilities() []string {
	return []string{"ai_ops_analysis", "gos_belief_engine"}
}

func (a *aiOpsGOSAgent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	query := ""
	if task != nil {
		query = strings.TrimSpace(task.Goal)
	}
	if task != nil && task.Input != nil {
		if raw, ok := task.Input["executable_query"].(string); ok && strings.TrimSpace(raw) != "" {
			query = strings.TrimSpace(raw)
		}
	}

	engine := buildAIOpsGoSEngine(ctx)
	if rt, ok := runtime.FromContext(ctx); ok && task != nil {
		engine.SetEmitter(func(emitCtx context.Context, message string, payload map[string]any) {
			rt.EmitInfo(emitCtx, task, a.Name(), message, payload)
		})
	}
	result := engine.Run(ctx, query)
	if result != nil && task != nil {
		result.TaskID = task.TaskID
		result.Agent = a.Name()
	}
	return result, nil
}

func getOrCreateAIOpsRuntime(ctx context.Context) (*runtime.Runtime, error) {
	dataDir := aiOpsRuntimeDataDir(ctx)
	aiOpsRuntimeMu.Lock()
	defer aiOpsRuntimeMu.Unlock()

	if rt, ok := aiOpsRuntimes[dataDir]; ok {
		return rt, nil
	}

	rt, err := buildAIOpsRuntime(ctx, dataDir)
	if err != nil {
		return nil, err
	}
	if err := registerAIOpsAgentsFn(rt); err != nil {
		return nil, err
	}
	aiOpsRuntimes[dataDir] = rt
	return rt, nil
}

// buildAIOpsRuntime 根据配置选择 Ledger 后端：
//   - aiops.ledger.backend = redis (推荐多实例部署)：跨实例共享 trace_id/task/result
//   - 默认 = file：本地落盘，单实例 / 共享 PVC 部署
//
// 多实例选 file 会出现「在 A 提交、5 秒后查询路由到 B → 404」，
// 因此在多副本配置下必须切到 redis。
func buildAIOpsRuntime(ctx context.Context, dataDir string) (*runtime.Runtime, error) {
	backend := strings.ToLower(strings.TrimSpace(func() string {
		v, _ := aiOpsConfigString(ctx, "aiops.ledger.backend")
		return v
	}()))
	switch backend {
	case "redis":
		retention := 24 * time.Hour
		if v, ok := aiOpsConfigInt(ctx, "aiops.ledger.retention_hours"); ok {
			retention = time.Duration(v) * time.Hour
		}
		maxTasks := 20000
		if v, ok := aiOpsConfigInt(ctx, "aiops.ledger.max_tasks"); ok {
			maxTasks = v
		}
		maxResults := 20000
		if v, ok := aiOpsConfigInt(ctx, "aiops.ledger.max_results"); ok {
			maxResults = v
		}
		redis := g.Redis()
		if redis == nil {
			g.Log().Warningf(ctx, "[aiops] ledger.backend=redis but g.Redis() is nil; falling back to persistent file backend")
			return newPersistentRuntime(dataDir)
		}
		prefix := "opscaption:ai:"
		if v, ok := aiOpsConfigString(ctx, "aiops.ledger.prefix"); ok {
			prefix = v
		}
		ledger := runtime.NewRedisLedger(redis, prefix, retention, maxTasks, maxResults)
		// artifacts 暂时仍走 file（H6 单独切 Redis）；bus 走 ledger bus 即可
		artifacts, err := runtime.NewFileArtifactStore(dataDir)
		if err != nil {
			return nil, fmt.Errorf("init artifact store: %w", err)
		}
		g.Log().Infof(ctx, "[aiops] using Redis ledger (prefix=%s, retention=%s, max_tasks=%d)", prefix, retention, maxTasks)
		return runtime.NewWithStores(ledger, runtime.NewLedgerBus(ledger), artifacts), nil
	default:
		return newPersistentRuntime(dataDir)
	}
}

func registerAIOpsAgents(rt *runtime.Runtime) error {
	if err := rt.Register(&aiOpsPlanAgent{}); err != nil {
		return err
	}
	return rt.Register(&aiOpsGOSAgent{})
}

func aiOpsRuntimeDataDir(ctx context.Context) string {
	v, err := gCfgGet(ctx, "multi_agent.data_dir")
	if err == nil && strings.TrimSpace(v) != "" {
		return v
	}
	return filepath.Join(".", "var", "runtime")
}

var gCfgGet = func(ctx context.Context, key string) (string, error) {
	v, err := g.Cfg().Get(ctx, key)
	if err != nil {
		return "", err
	}
	return v.String(), nil
}

func selectAIOpsAgentName(ctx context.Context) string {
	agentName, _, _ := resolveAIOpsAgentName(ctx)
	return agentName
}

func resolveAIOpsAgentName(ctx context.Context) (string, bool, string) {
	engine, configured := requestedAIOpsEngine(ctx)
	explicitRequest := configured
	if !configured {
		engine, configured = aiOpsConfigString(ctx, "aiops.engine")
	}
	if !configured {
		return aiOpsPlanAgentName, true, ""
	}
	agentName := normalizeAIOpsAgentName(engine)
	if agentName != aiOpsGOSAgentName {
		return aiOpsPlanAgentName, true, ""
	}
	enabled, ok := aiOpsConfigBool(ctx, "aiops.gos.enabled")
	if !ok || !enabled {
		if explicitRequest {
			return aiOpsGOSAgentName, false, "GoS 引擎已在配置中关闭，当前请求没有实际进入 GoS 信念推理链路。"
		}
		return aiOpsPlanAgentName, true, ""
	}
	return aiOpsGOSAgentName, true, ""
}

func normalizeAIOpsAgentName(engine string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "gos", "gos_engine", aiOpsGOSAgentName:
		return aiOpsGOSAgentName
	default:
		return aiOpsPlanAgentName
	}
}

func WithAIOpsEngine(ctx context.Context, engine string) context.Context {
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		return ctx
	}
	return context.WithValue(ctx, aiOpsEngineContextKey{}, engine)
}

func requestedAIOpsEngine(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	engine, ok := ctx.Value(aiOpsEngineContextKey{}).(string)
	if !ok || strings.TrimSpace(engine) == "" {
		return "", false
	}
	return strings.TrimSpace(engine), true
}

func newAIOpsGoSEngine(ctx context.Context) *gos_engine.GoSEngine {
	cfg := LoadAIOpsGOSConfig(ctx)
	engine := gos_engine.NewGoSEngine(cfg, aiOpsGOSLogger{})
	toolReg := experts.NewToolRegistry()
	registerAIOpsGOSTools(toolReg)
	for _, expertCfg := range cfg.Experts {
		registerAIOpsGOSExpert(engine, cfg, toolReg, expertCfg)
	}
	return engine
}

// LoadAIOpsGOSConfig returns the effective GoS runtime configuration. The
// standalone evaluator uses the same loader so real compare exercises the
// configured production candidate instead of DefaultConfig compatibility mode.
func LoadAIOpsGOSConfig(ctx context.Context) *gos_engine.Config {
	return loadAIOpsGOSConfig(ctx)
}

func loadAIOpsGOSConfig(ctx context.Context) *gos_engine.Config {
	cfg := gos_engine.DefaultConfig()
	if v, ok := aiOpsConfigBool(ctx, "aiops.gos.enabled"); ok {
		cfg.Enabled = v
	}
	if v, ok := aiOpsConfigString(ctx, "aiops.gos.model_path"); ok {
		cfg.ModelPath = v
	}
	if v, ok := aiOpsConfigFloat(ctx, "aiops.gos.temperature"); ok {
		cfg.Temperature = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.max_tokens"); ok {
		cfg.MaxTokens = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.evidence_max_chars"); ok {
		cfg.EvidenceMaxChars = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.session_max_steps"); ok {
		cfg.SessionMaxSteps = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.max_retrieval_steps"); ok {
		cfg.MaxRetrievalSteps = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.call_timeout_ms"); ok {
		cfg.CallTimeoutMs = v
	}
	if v, ok := aiOpsConfigFloat(ctx, "aiops.gos.fsm.gap_delta"); ok {
		cfg.FSM.GapDelta = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.fsm.min_support"); ok {
		cfg.FSM.MinSupport = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.fsm.max_steps"); ok {
		cfg.FSM.MaxSteps = v
	}
	if v, ok := aiOpsConfigFloat(ctx, "aiops.gos.fsm.min_confidence"); ok {
		cfg.FSM.MinConfidence = v
	}
	if v, ok := aiOpsConfigFloat(ctx, "aiops.gos.confidence.support_weight"); ok {
		cfg.Confidence.SupportWeight = v
	}
	if v, ok := aiOpsConfigFloat(ctx, "aiops.gos.confidence.refute_weight"); ok {
		cfg.Confidence.RefuteWeight = v
	}
	if v, ok := aiOpsConfigBool(ctx, "aiops.gos.confidence.deduplicate"); ok {
		cfg.Confidence.Deduplicate = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.graph.checkpoint_interval"); ok {
		cfg.Graph.CheckpointInterval = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.graph.max_nodes"); ok {
		cfg.Graph.MaxNodes = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.graph.max_edges"); ok {
		cfg.Graph.MaxEdges = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.graph.max_depth"); ok {
		cfg.Graph.MaxDepth = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.graph.max_snapshots"); ok {
		cfg.Graph.MaxSnapshots = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.graph.max_deltas"); ok {
		cfg.Graph.MaxDeltas = v
	}
	if v, ok := aiOpsConfigBool(ctx, "aiops.gos.structured_cognition.enabled"); ok {
		cfg.StructuredCognition.Enabled = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.structured_cognition.call_timeout_ms"); ok {
		cfg.StructuredCognition.CallTimeoutMs = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.structured_cognition.max_hypotheses"); ok {
		cfg.StructuredCognition.MaxHypotheses = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.structured_cognition.max_observations"); ok {
		cfg.StructuredCognition.MaxObservations = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.structured_cognition.max_plan_items"); ok {
		cfg.StructuredCognition.MaxPlanItems = v
	}
	if v, err := g.Cfg().Get(ctx, "aiops.gos.structured_cognition.plan_budget"); err == nil {
		var budget gos_engine.PlanBudgetConfig
		if err := v.Scan(&budget); err == nil {
			cfg.StructuredCognition.PlanBudget = budget
		}
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.execution.max_concurrent_experts"); ok {
		cfg.Execution.MaxConcurrentExperts = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.execution.no_progress_round_limit"); ok {
		cfg.Execution.NoProgressRoundLimit = v
	}
	if v, ok := aiOpsConfigFloat(ctx, "aiops.gos.report.conflict_strength_threshold"); ok {
		cfg.Report.ConflictStrengthThreshold = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.report.max_evidence_items"); ok {
		cfg.Report.MaxEvidenceItems = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.report.evidence_snippet_max_chars"); ok {
		cfg.Report.EvidenceSnippetMaxChars = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.report.max_next_actions"); ok {
		cfg.Report.MaxNextActions = v
	}
	if v, err := g.Cfg().Get(ctx, "aiops.gos.experts"); err == nil {
		var expertConfigs []gos_engine.ExpertConfig
		if err := v.Scan(&expertConfigs); err == nil && len(expertConfigs) > 0 {
			cfg.Experts = expertConfigs
		}
	}
	if v, ok := aiOpsConfigBool(ctx, "aiops.gos.state_conversion.enabled"); ok {
		cfg.StateConversion.Enabled = v
	}
	if v, ok := aiOpsConfigInt(ctx, "aiops.gos.state_conversion.max_depth"); ok {
		cfg.StateConversion.MaxDepth = v
	}
	if v, ok := aiOpsConfigFloat(ctx, "aiops.gos.state_conversion.tie_epsilon"); ok {
		cfg.StateConversion.TieEpsilon = v
	}
	if v, err := g.Cfg().Get(ctx, "aiops.gos.state_conversion.refinement_rules"); err == nil {
		var rules []gos_engine.RefinementRule
		if err := v.Scan(&rules); err == nil && len(rules) > 0 {
			cfg.StateConversion.RefinementRules = rules
		}
	}
	return cfg
}

func registerAIOpsGOSTools(toolReg *experts.ToolRegistry) {
	if t := aitools.NewQueryInternalDocsTool(); t != nil {
		toolReg.Register("query_internal_docs", t)
	}
	if t := aitools.NewPrometheusInstantQueryTool(); t != nil {
		toolReg.Register("query_prometheus_instant", t)
	}
	if t := aitools.NewPrometheusAlertsQueryTool(); t != nil {
		toolReg.Register("query_prometheus_alerts", t)
	}
	logTools, err := aitools.GetLogMcpTool()
	registeredLog := false
	for _, t := range logTools {
		invokable, ok := t.(tool.InvokableTool)
		if !ok {
			continue
		}
		info, infoErr := invokable.Info(context.Background())
		if infoErr != nil || info == nil || info.Name == "" {
			continue
		}
		toolReg.Register(info.Name, invokable)
		if info.Name == "query_logs" {
			registeredLog = true
		}
	}
	if !registeredLog {
		reason := "query_logs invokable tool is unavailable"
		if err != nil {
			reason = err.Error()
		}
		unavailTool := aitools.NewUnavailableLogQueryTool(reason)
		if unavailTool != nil {
			toolReg.Register("query_logs", unavailTool)
		}
	}
}

// RegisterAIOpsGOSTools exposes the production GoS tool wiring to the standalone
// evaluator so real-profile runs exercise the same registered capabilities.
func RegisterAIOpsGOSTools(toolReg *experts.ToolRegistry) {
	registerAIOpsGOSTools(toolReg)
}

func registerAIOpsGOSExpert(engine *gos_engine.GoSEngine, cfg *gos_engine.Config, toolReg *experts.ToolRegistry, ec gos_engine.ExpertConfig) {
	if strings.TrimSpace(ec.Name) == "" {
		return
	}
	if ec.MaxRetrievalSteps <= 0 {
		ec.MaxRetrievalSteps = cfg.MaxRetrievalSteps
	}
	if ec.MaxRetrievalSteps <= 0 {
		ec.MaxRetrievalSteps = 3
	}
	if len(ec.Tools) == 0 {
		ec.Tools = []string{"query_logs", "query_internal_docs"}
	}
	runtimeCfg := experts.ExpertRuntimeConfig{
		Name:              ec.Name,
		Description:       ec.Description,
		ToolNames:         ec.Tools,
		MaxRetrievalSteps: ec.MaxRetrievalSteps,
		ModelPath:         cfg.ModelPath,
		Temperature:       cfg.Temperature,
		MaxTokens:         cfg.MaxTokens,
		EvidenceMaxChars:  cfg.EvidenceMaxChars,
		CallTimeout:       time.Duration(cfg.CallTimeoutMs) * time.Millisecond,
		ChatModelFactory:  models.OpenAIChatModelFactory(cfg.ModelPath),
		ExecutionBudget: experts.ExecutionBudget{
			LLMCalls:          ec.Budget.LLMCalls,
			ToolCalls:         ec.Budget.ToolCalls,
			RAGCalls:          ec.Budget.RAGCalls,
			Timeout:           time.Duration(ec.Budget.TimeoutMs) * time.Millisecond,
			MaxRetrievalSteps: ec.Budget.MaxRetrievalSteps,
			MaxOutputTokens:   ec.Budget.MaxOutputTokens,
		},
	}
	switch strings.ToLower(strings.TrimSpace(ec.Name)) {
	case "linux_sre":
		engine.RegisterExpert(ec.Name, experts.NewLinuxSREExpert(runtimeCfg, toolReg))
	case "network_sre":
		engine.RegisterExpert(ec.Name, experts.NewNetworkSREExpert(runtimeCfg, toolReg))
	case "database_sre":
		engine.RegisterExpert(ec.Name, experts.NewDatabaseSREExpert(runtimeCfg, toolReg))
	default:
		engine.RegisterExpert(ec.Name, experts.NewBaseExpert(runtimeCfg, toolReg))
	}
}

// RegisterAIOpsGOSExpert exposes the production expert and budget wiring to the
// standalone evaluator.
func RegisterAIOpsGOSExpert(engine *gos_engine.GoSEngine, cfg *gos_engine.Config, toolReg *experts.ToolRegistry, ec gos_engine.ExpertConfig) {
	registerAIOpsGOSExpert(engine, cfg, toolReg, ec)
}

type aiOpsGOSLogger struct{}

func (aiOpsGOSLogger) Info(msg string, keysAndValues ...interface{}) {
	g.Log().Info(context.Background(), append([]interface{}{msg}, keysAndValues...)...)
}

func (aiOpsGOSLogger) Error(msg string, keysAndValues ...interface{}) {
	g.Log().Error(context.Background(), append([]interface{}{msg}, keysAndValues...)...)
}
