package chat_pipeline

import (
	"SuperBizAgent/internal/ai/agent/skillspecialists/knowledge"
	"SuperBizAgent/internal/ai/agent/skillspecialists/logs"
	"SuperBizAgent/internal/ai/agent/skillspecialists/metrics"
	"SuperBizAgent/internal/ai/events"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/ai/tools"
	"context"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/gogf/gf/v2/frame/g"
)

var chatDisclosure = skills.NewProgressiveDisclosure(
	[]*skills.Registry{
		logs.SkillRegistry(),
		metrics.SkillRegistry(),
		knowledge.SkillRegistry(),
	},
	tools.BuildTieredTools(),
)

// chatToolEmitter 工具事件发射器（由 ChatStream 设置）
var chatToolEmitter events.Emitter

// chatToolTraceID 工具事件 traceID（由 ChatStream 设置）
var chatToolTraceID string

// SetChatToolEmitter 设置工具事件发射器（由 ChatStream 调用）
func SetChatToolEmitter(emitter events.Emitter, traceID string) {
	chatToolEmitter = emitter
	chatToolTraceID = traceID
}

func NormalizeSelectedSkillIDs(selectedSkillIDs []string) []string {
	selectedSkills := chatDisclosure.ResolveSelectedSkills(selectedSkillIDs)
	if len(selectedSkills) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(selectedSkills))
	for _, selected := range selectedSkills {
		if selected.Name == "" {
			continue
		}
		normalized = append(normalized, selected.Name)
	}
	return normalized
}

func newReactAgentLambda(ctx context.Context) (lba *compose.Lambda, err error) {
	return newReactAgentLambdaWithQuery(ctx, "")
}

func newReactAgentLambdaWithQuery(ctx context.Context, query string) (lba *compose.Lambda, err error) {
	config := &react.AgentConfig{
		MaxStep:            25,
		ToolReturnDirectly: map[string]struct{}{}}
	chatModelIns11, err := newChatModel(ctx)
	if err != nil {
		return nil, err
	}
	config.ToolCallingModel = chatModelIns11

	var disclosed skills.DisclosureResult
	selectedSkillIDs := skills.SelectedSkillIDsFromContext(ctx)
	if query != "" {
		disclosed = chatDisclosure.Disclose(query, selectedSkillIDs)
		config.ToolsConfig.Tools = disclosed.Tools
		g.Log().Infof(ctx, "[Chat] progressive disclosure: query=%q selected=%v domains=%v tools=%d (L0=%d L1=%d)",
			query, selectedSkillIDs, disclosed.MatchedDomains, len(disclosed.Tools), disclosed.DisclosedTier[skills.TierAlwaysOn],
			disclosed.DisclosedTier[skills.TierSkillGate])
	} else {
		config.ToolsConfig.Tools = chatDisclosure.AllTools()
	}

	// 如果配置了事件发射器，包装工具以支持 before/after 拦截和事件发射
	if chatToolEmitter != nil {
		config.ToolsConfig.Tools = events.WrapTools(
			config.ToolsConfig.Tools,
			chatToolEmitter,
			chatToolTraceID,
			nil,                               // beforeToolCall: 暂不启用
			events.SummaryAfterToolCall(4000), // afterToolCall: 截断过长结果，减少 token 消耗
		)
	}

	ins, err := react.NewAgent(ctx, config)
	if err != nil {
		return nil, err
	}
	lba, err = compose.AnyLambda(ins.Generate, ins.Stream, nil, nil)
	if err != nil {
		return nil, err
	}
	return lba, nil
}
