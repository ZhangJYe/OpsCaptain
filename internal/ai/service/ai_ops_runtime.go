package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/agent/gos_engine"
	"SuperBizAgent/internal/ai/agent/plan_execute_replan"
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

	content, planDetail, err := buildPlanAgent(ctx, query)
	if rt, ok := runtime.FromContext(ctx); ok && task != nil {
		for _, step := range planDetail {
			step = strings.TrimSpace(step)
			if step == "" {
				continue
			}
			rt.EmitInfo(ctx, task, a.Name(), step, nil)
		}
	}
	if err != nil {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      a.Name(),
			Status:     protocol.ResultStatusFailed,
			Summary:    fmt.Sprintf("plan-execute-replan failed: %v", err),
			Confidence: 0,
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
	}, nil
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

	rt, err := newPersistentRuntime(dataDir)
	if err != nil {
		return nil, err
	}
	if err := registerAIOpsAgentsFn(rt); err != nil {
		return nil, err
	}
	aiOpsRuntimes[dataDir] = rt
	return rt, nil
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
	engine, configured := requestedAIOpsEngine(ctx)
	if !configured {
		engine, configured = aiOpsConfigString(ctx, "aiops.engine")
	}
	if !configured {
		return aiOpsPlanAgentName
	}
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine != "gos" && engine != "gos_engine" {
		return aiOpsPlanAgentName
	}
	enabled, ok := aiOpsConfigBool(ctx, "aiops.gos.enabled")
	if !ok || !enabled {
		return aiOpsPlanAgentName
	}
	return aiOpsGOSAgentName
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
	cfg := loadAIOpsGOSConfig(ctx)
	engine := gos_engine.NewGoSEngine(cfg, aiOpsGOSLogger{})
	toolReg := experts.NewToolRegistry()
	registerAIOpsGOSTools(toolReg)
	for _, expertCfg := range cfg.Experts {
		registerAIOpsGOSExpert(engine, cfg, toolReg, expertCfg)
	}
	return engine
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
	return cfg
}

func registerAIOpsGOSTools(toolReg *experts.ToolRegistry) {
	toolReg.Register("query_internal_docs", aitools.NewQueryInternalDocsTool())
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
		toolReg.Register("query_logs", aitools.NewUnavailableLogQueryTool(reason))
	}
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
		CallTimeout:       time.Duration(cfg.CallTimeoutMs) * time.Millisecond,
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

type aiOpsGOSLogger struct{}

func (aiOpsGOSLogger) Info(msg string, keysAndValues ...interface{}) {
	g.Log().Info(context.Background(), append([]interface{}{msg}, keysAndValues...)...)
}

func (aiOpsGOSLogger) Error(msg string, keysAndValues ...interface{}) {
	g.Log().Error(context.Background(), append([]interface{}{msg}, keysAndValues...)...)
}
