package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"SuperBizAgent/internal/ai/agent/plan_execute_replan"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"

	"github.com/gogf/gf/v2/frame/g"
)

const aiOpsPlanAgentName = "aiops_plan_execute_replan"

var (
	aiOpsRuntimeMu        sync.Mutex
	aiOpsRuntimes         = make(map[string]*runtime.Runtime)
	registerAIOpsAgentsFn = registerAIOpsAgents
	buildPlanAgent        = plan_execute_replan.BuildPlanAgent
)

type aiOpsPlanAgent struct{}

func (a *aiOpsPlanAgent) Name() string {
	return aiOpsPlanAgentName
}

func (a *aiOpsPlanAgent) Capabilities() []string {
	return []string{"ai_ops_analysis", "plan_execute_replan"}
}

func (a *aiOpsPlanAgent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	query := strings.TrimSpace(task.Goal)
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
	return rt.Register(&aiOpsPlanAgent{})
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
