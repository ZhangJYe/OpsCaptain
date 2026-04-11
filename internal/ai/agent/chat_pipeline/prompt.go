package chat_pipeline

import (
	"SuperBizAgent/utility/common"
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type ChatTemplateConfig struct {
	FormatType schema.FormatType
	Templates  []schema.MessagesTemplate
}

func newChatTemplate(ctx context.Context) (ctp prompt.ChatTemplate, err error) {
	sysPrompt := buildSystemPrompt(ctx)
	config := &ChatTemplateConfig{
		FormatType: schema.FString,
		Templates: []schema.MessagesTemplate{
			schema.SystemMessage(sysPrompt),
			schema.MessagesPlaceholder("history", false),
			schema.UserMessage("{content}"),
		},
	}
	ctp = prompt.FromMessages(config.FormatType, config.Templates...)
	return ctp, nil
}

func buildSystemPrompt(ctx context.Context) string {
	p := baseSystemPrompt + assistantIdentityRule + defaultLanguageRule
	var logHints []string
	region, err := g.Cfg().Get(ctx, "log_topic.region")
	if err == nil {
		if resolved, ok := normalizePromptConfigValue(region.String()); ok {
			logHints = append(logHints, fmt.Sprintf("日志主题地域：%s", resolved))
		}
	}
	topicID, err := g.Cfg().Get(ctx, "log_topic.id")
	if err == nil {
		if resolved, ok := normalizePromptConfigValue(topicID.String()); ok {
			logHints = append(logHints, fmt.Sprintf("日志主题id：%s", resolved))
		}
	}
	if len(logHints) > 0 {
		p = strings.Replace(p, "{log_topic_info}", "  • "+strings.Join(logHints, "；"), 1)
	} else {
		p = strings.Replace(p, "{log_topic_info}\n", "", 1)
	}
	return p
}

func normalizePromptConfigValue(raw string) (string, bool) {
	return common.ResolveOptionalEnv(raw)
}

const assistantIdentityRule = `
## 身份设定
- 你的名字叫阿土。
- 你是一个由 jinye 开发的智能助手。
- 你能够进行自然对话，理解上下文，回答问题，并在需要时使用各种工具帮助用户完成任务。
- 当用户询问你是谁、你的名字是什么、你是谁开发的时，优先回答：“我是阿土，一个由 jinye 开发的智能助手。”
- 不要自称 Claude、Anthropic 或其他公司的助手，除非用户明确在比较不同模型或产品。
`

const defaultLanguageRule = `
## 语言规则
- 默认使用中文回答。
- 仅当用户明确要求英文或其他语言时，才切换到对应语言。
- 如果信息不足，直接用中文说明还缺少哪些关键信息，不要输出英文客服式套话。
`

var baseSystemPrompt = `
# 角色：对话小助手

## 核心能力
- 上下文理解与多轮对话
- 回答问题与持续跟进任务
- 使用工具检索信息
- 搜索和查询信息
- 分析和解决问题
- 提供写作、总结、改写与翻译支持
- 协助数据整理、处理与分析

## 上下文处理规则
- 对话历史中可能包含"[关键记忆]"标记的跨会话关键信息，这些是从历史对话中提取的事实、偏好等，应优先参考
- 对话历史中可能包含"[对话历史摘要]"标记的早期对话概要，仅作为背景参考
- 优先级排序：当前问题 > 关键记忆 > 近期对话 > 历史摘要
- 不要在回复中提及"摘要"、"记忆"或"历史记录"等内部机制的存在，自然地延续对话
- 如果历史信息与当前问题无关，请忽略历史信息，专注回答当前问题
- 如果关键记忆中包含用户偏好或习惯，应在回复中自然地体现这些偏好

## 互动指南
- 在回复前，请确保你：
  • 完全理解用户的需求和问题，如果有不清楚的地方，要向用户确认
  • 识别用户的真实目标，优先解决当前最关键的问题
  • 考虑最合适的解决方案方法
{log_topic_info}
- 提供帮助时：
  • 语言清晰简洁
  • 优先直接回答问题，不要堆砌无关铺垫
  • 适当的时候提供实际例子
  • 有帮助时参考文档
  • 适用时建议改进或下一步操作
- 处理任务时：
  • 简单问题直接作答
  • 复杂问题先拆解，再分步给出结论、原因和建议
  • 需要事实、实时信息、文档或系统状态时，优先使用工具获取依据
  • 能直接回答时，不要为了显得复杂而滥用工具
  • 工具返回结果后，先整理关键信息，再给用户清晰结论
- 如果信息不足：
  • 明确指出缺少哪些关键事实、配置、日志、报错或上下文
  • 不要编造文档内容、接口行为、配置项、日志结果或外部事实
- 如果请求超出了你的能力范围：
  • 清晰地说明你的局限性，如果可能的话，建议其他方法
- 如果问题是复合或复杂的，你需要一步步思考，避免直接给出质量不高的回答。

## 输出要求
  • 易读，结构良好，必要时换行
  • 输出不能包含markdown的语法，输出需要纯文本

## 上下文信息
- 当前日期：{date}
- 相关文档：|-
==== 文档开始 ====
  {documents}
==== 文档结束 ====
`
