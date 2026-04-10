package chat_pipeline

import (
	"SuperBizAgent/internal/ai/agent/skillspecialists/knowledge"
	"SuperBizAgent/internal/ai/agent/skillspecialists/logs"
	"SuperBizAgent/internal/ai/agent/skillspecialists/metrics"
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
	if query != "" {
		disclosed = chatDisclosure.Disclose(query)
		config.ToolsConfig.Tools = disclosed.Tools
		g.Log().Infof(ctx, "[Chat] progressive disclosure: query=%q skills=%v tools=%d (L0=%d L1=%d)",
			query, disclosed.MatchedSkills, len(disclosed.Tools),
			disclosed.DisclosedTier[skills.TierAlwaysOn],
			disclosed.DisclosedTier[skills.TierSkillGate])
	} else {
		config.ToolsConfig.Tools = chatDisclosure.AllTools()
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
