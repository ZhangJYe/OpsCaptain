package plan_execute_replan

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/gogf/gf/v2/frame/g"
)

func BuildPlanAgent(ctx context.Context, query string) (string, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	planAgent, err := NewPlanner(ctx)
	if err != nil {
		return "", []string{}, err
	}
	executeAgent, err := NewExecutor(ctx)
	if err != nil {
		return "", []string{}, err
	}
	replanAgent, err := NewRePlanAgent(ctx)
	if err != nil {
		return "", []string{}, err
	}
	planExecuteAgent, err := planexecute.New(ctx, &planexecute.Config{
		Planner:       planAgent,
		Executor:      executeAgent,
		Replanner:     replanAgent,
		MaxIterations: 5,
	})
	if err != nil {
		return "", []string{}, fmt.Errorf("build PlanExecuteAgent Error: %w", err)
	}
	r := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: planExecuteAgent,
	})
	iter := r.Query(ctx, query)

	var lastAnalysis string
	var detail []string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Output == nil {
			continue
		}
		msg, _, err := adk.GetMessage(event)
		if err != nil {
			continue
		}
		g.Log().Debugf(ctx, "[AIOps] step: %s", msg.String())
		detail = append(detail, msg.String())

		if isAnalysisMessage(msg) {
			lastAnalysis = msg.Content
		}
	}

	if lastAnalysis == "" {
		return "", detail, fmt.Errorf("no analysis conclusion found in event stream")
	}
	return lastAnalysis, detail, nil
}

func isAnalysisMessage(msg adk.Message) bool {
	if msg.Role != "assistant" {
		return false
	}
	if len(msg.Content) < 20 {
		return false
	}
	if len(msg.ToolCalls) > 0 {
		return false
	}
	if isPlanJSON(msg.Content) {
		return false
	}
	return true
}

func isPlanJSON(content string) bool {
	s := strings.TrimSpace(content)
	if len(s) < 2 {
		return false
	}
	if s[0] != '{' {
		return false
	}
	return len(s) > 10 && (strings.Contains(s, `"steps"`) || strings.Contains(s, `"plan"`))
}
