package events

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// LLMValidator 使用 LLM 进行高级防幻觉校验
type LLMValidator struct {
	modelFactory func(ctx context.Context) (model.ToolCallingChatModel, error)
	config       *LLMValidationConfig
}

// LLMValidationResult LLM 校验结果
type LLMValidationResult struct {
	OmissionWarnings []string
	AccuracyWarnings []string
	RawLLMResponse   string
}

// NewLLMValidator 创建 LLM 校验器
func NewLLMValidator(modelFactory func(ctx context.Context) (model.ToolCallingChatModel, error), config *LLMValidationConfig) *LLMValidator {
	return &LLMValidator{
		modelFactory: modelFactory,
		config:       config,
	}
}

// Validate 使用 LLM 校验输出
func (v *LLMValidator) Validate(ctx context.Context, output string, toolResults []string) *LLMValidationResult {
	result := &LLMValidationResult{}

	if v.config == nil || !v.config.Enabled {
		return result
	}

	// 构建超时 context
	timeout := time.Duration(v.config.TimeoutMs) * time.Millisecond
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	combinedToolResults := strings.Join(toolResults, "\n---\n")

	// 遗漏信息检测
	if v.config.OmissionDetection && len(toolResults) > 0 {
		omissionWarnings := v.detectOmissions(ctx, output, combinedToolResults)
		result.OmissionWarnings = omissionWarnings
	}

	// 准确性校验
	if v.config.AccuracyCheck && len(toolResults) > 0 {
		accuracyWarnings := v.checkAccuracy(ctx, output, combinedToolResults)
		result.AccuracyWarnings = accuracyWarnings
	}

	return result
}

// detectOmissions 检测输出中遗漏的重要信息
func (v *LLMValidator) detectOmissions(ctx context.Context, output string, toolResults string) []string {
	prompt := fmt.Sprintf(`你是 AIOps 输出质量审查员。请对比工具返回的数据和 AI 的回答，找出工具数据中重要但回答中未提及的信息。

## 工具返回的数据
%s

## AI 的回答
%s

## 任务
1. 列出工具数据中的关键发现（指标异常、错误、告警等）
2. 检查 AI 的回答是否提及了这些关键发现
3. 列出遗漏的重要信息

## 输出格式
如果无遗漏，输出：无遗漏
如果有遗漏，每行一条，格式：[遗漏] 具体遗漏内容

只输出结果，不要解释。`, toolResults, output)

	return v.callLLMForWarnings(ctx, prompt)
}

// checkAccuracy 检查输出中的声明是否与工具数据一致
func (v *LLMValidator) checkAccuracy(ctx context.Context, output string, toolResults string) []string {
	prompt := fmt.Sprintf(`你是 AIOps 输出质量审查员。请检查 AI 回答中的每个具体声明（指标值、状态、结论）是否与工具返回的数据一致。

## 工具返回的数据
%s

## AI 的回答
%s

## 任务
1. 提取 AI 回答中的具体声明（数字、状态、结论）
2. 对照工具数据验证每个声明
3. 列出不一致或无依据的声明

## 输出格式
如果全部准确，输出：全部准确
如果有问题，每行一条，格式：[问题] 具体问题描述

只输出结果，不要解释。`, toolResults, output)

	return v.callLLMForWarnings(ctx, prompt)
}

// callLLMForWarnings 调用 LLM 获取警告列表
func (v *LLMValidator) callLLMForWarnings(ctx context.Context, prompt string) []string {
	if v.modelFactory == nil {
		return nil
	}

	cm, err := v.modelFactory(ctx)
	if err != nil {
		return nil
	}

	maxTokens := v.config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 512
	}

	messages := []*schema.Message{
		{Role: schema.System, Content: "你是 AIOps 输出质量审查员。只输出审查结果，不要解释。"},
		{Role: schema.User, Content: prompt},
	}

	resp, err := cm.Generate(ctx, messages)
	if err != nil {
		return nil
	}

	if resp == nil || resp.Content == "" {
		return nil
	}

	return parseWarnings(resp.Content)
}

// parseWarnings 解析 LLM 返回的警告
func parseWarnings(content string) []string {
	lines := strings.Split(content, "\n")
	var warnings []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 跳过"无遗漏"、"全部准确"等正面回复
		if strings.Contains(line, "无遗漏") || strings.Contains(line, "全部准确") {
			return nil
		}
		if strings.HasPrefix(line, "[遗漏]") || strings.HasPrefix(line, "[问题]") {
			warnings = append(warnings, line)
		}
	}
	return warnings
}
