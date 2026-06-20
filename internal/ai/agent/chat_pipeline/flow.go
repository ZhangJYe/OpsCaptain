package chat_pipeline

import (
	"SuperBizAgent/internal/ai/events"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/ai/skills/domains/knowledge"
	"SuperBizAgent/internal/ai/skills/domains/logs"
	"SuperBizAgent/internal/ai/skills/domains/metrics"
	"SuperBizAgent/internal/ai/tools"
	"SuperBizAgent/internal/consts"
	"context"
	"io"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

var (
	chatDisclosureOnce sync.Once
	chatDisclosureIns  *skills.ProgressiveDisclosure

	// Lazy-initialized user tools dependencies; set via SetUserToolDeps before first chat.
	userToolStoreDeps skills.UserSkillStore
	dynamicMCPRegDeps *tools.DynamicMCPRegistry
)

// SetUserToolDeps configures user tool dependencies for progressive disclosure.
// Call this before any chat request. Nil values are safe.
func SetUserToolDeps(store skills.UserSkillStore, reg *tools.DynamicMCPRegistry) {
	userToolStoreDeps = store
	dynamicMCPRegDeps = reg
}

func getChatDisclosure() *skills.ProgressiveDisclosure {
	chatDisclosureOnce.Do(func() {
		chatDisclosureIns = skills.NewProgressiveDisclosure(
			[]*skills.Registry{
				logs.SkillRegistry(),
				metrics.SkillRegistry(),
				knowledge.SkillRegistry(),
			},
			tools.BuildTieredTools(context.Background(), userToolStoreDeps, dynamicMCPRegDeps),
		)
	})
	return chatDisclosureIns
}

type chatToolEmitterContextKey struct{}

type chatToolEmitterConfig struct {
	emitter events.Emitter
	traceID string
}

func WithChatToolEmitter(ctx context.Context, emitter events.Emitter, traceID string) context.Context {
	if emitter == nil {
		return ctx
	}
	return context.WithValue(ctx, chatToolEmitterContextKey{}, chatToolEmitterConfig{
		emitter: emitter,
		traceID: traceID,
	})
}

func chatToolEmitterFromContext(ctx context.Context) (events.Emitter, string, bool) {
	cfg, ok := ctx.Value(chatToolEmitterContextKey{}).(chatToolEmitterConfig)
	if !ok || cfg.emitter == nil {
		return nil, "", false
	}
	return cfg.emitter, cfg.traceID, true
}

func NormalizeSelectedSkillIDs(selectedSkillIDs []string) []string {
	selectedSkills := getChatDisclosure().ResolveSelectedSkills(selectedSkillIDs)
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

func fullStreamToolCallChecker(_ context.Context, sr *schema.StreamReader[*schema.Message]) (bool, error) {
	defer sr.Close()
	for {
		msg, err := sr.Recv()
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if len(msg.ToolCalls) > 0 {
			return true, nil
		}
	}
}

func newReactAgentLambda(ctx context.Context) (lba *compose.Lambda, err error) {
	return newReactAgentLambdaWithQuery(ctx, "")
}

func newReactAgentLambdaWithQuery(ctx context.Context, query string) (lba *compose.Lambda, err error) {
	config := &react.AgentConfig{
		MaxStep:               chatConfigInt(ctx, "chat.react.max_step", 25),
		ToolReturnDirectly:    map[string]struct{}{},
		StreamToolCallChecker: fullStreamToolCallChecker,
	}
	chatModelIns11, err := newChatModelWithQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	config.ToolCallingModel = chatModelIns11

	// Restrict tools when injection risk is elevated
	riskLevel, _ := ctx.Value(consts.CtxKeyInjectionRiskLevel).(string)
	if riskLevel == "suspicious" {
		config.ToolsConfig.Tools = getChatDisclosure().OnlyAlwaysOnTools()
		g.Log().Infof(ctx, "[Chat] injection risk=suspicious, restricted to always-on tools only (%d tools)", len(config.ToolsConfig.Tools))
	} else {
		var disclosed skills.DisclosureResult
		selectedSkillIDs := skills.SelectedSkillIDsFromContext(ctx)
		if query != "" {
			disclosed = getChatDisclosure().Disclose(query, selectedSkillIDs)
			config.ToolsConfig.Tools = disclosed.Tools
			g.Log().Infof(ctx, "[Chat] progressive disclosure: query=%q selected=%v domains=%v tools=%d (L0=%d L1=%d)",
				query, selectedSkillIDs, disclosed.MatchedDomains, len(disclosed.Tools), disclosed.DisclosedTier[skills.TierAlwaysOn],
				disclosed.DisclosedTier[skills.TierSkillGate])
		} else {
			config.ToolsConfig.Tools = getChatDisclosure().AllTools()
		}
	}

	if emitter, traceID, ok := chatToolEmitterFromContext(ctx); ok {
		config.ToolsConfig.Tools = events.WrapTools(
			config.ToolsConfig.Tools,
			emitter,
			traceID,
			events.ValidateBeforeToolCall(), // 参数基础校验
			events.CompressAfterToolCall(func() string { return query }, chatConfigInt(ctx, "events.tool_summary_max_len", 4000)),
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

func chatConfigInt(ctx context.Context, key string, fallback int) int {
	v, err := g.Cfg().Get(ctx, key)
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	return fallback
}
