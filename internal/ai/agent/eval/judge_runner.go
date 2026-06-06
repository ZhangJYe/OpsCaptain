package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/models"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// JudgeRunner uses an LLM to score diagnostic outputs on 4 dimensions.
type JudgeRunner struct {
	model   model.ToolCallingChatModel
	timeout time.Duration
}

// NewJudgeRunner creates a JudgeRunner using the fast chat model.
func NewJudgeRunner(ctx context.Context) (*JudgeRunner, error) {
	m, err := models.OpenAIForGLMFast(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create judge model: %w", err)
	}
	return &JudgeRunner{
		model:   m,
		timeout: 30 * time.Second,
	}, nil
}

// NewJudgeRunnerWithModel creates a JudgeRunner with a custom model (for testing).
func NewJudgeRunnerWithModel(m model.ToolCallingChatModel, timeout time.Duration) *JudgeRunner {
	return &JudgeRunner{model: m, timeout: timeout}
}

// Score evaluates a diagnostic output on 4 dimensions (1-5 scale).
func (j *JudgeRunner) Score(ctx context.Context, query, answer string, toolData []string) (*DiagScores, error) {
	if j.model == nil {
		return nil, fmt.Errorf("judge model not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()

	toolDataStr := strings.Join(toolData, "\n- ")
	if toolDataStr == "" {
		toolDataStr = "(无工具数据)"
	}

	prompt := fmt.Sprintf(`你是一个运维诊断质量评估专家。请对以下诊断结果进行评分。

## 用户问题
%s

## 诊断结果
%s

## 工具数据
- %s

## 评分维度（1-5 分）
1. 正确性 (Correctness)：诊断结论是否基于工具数据，是否有幻觉
2. 完整性 (Completeness)：是否覆盖了工具数据中的关键发现
3. 连贯性 (Coherence)：逻辑是否清晰，论述是否自洽
4. 可操作性 (Actionability)：是否给出了具体可执行的排查建议
5. 总体评分 (Overall)

请严格按以下 JSON 格式输出，不要输出其他内容：
{"correctness": N, "completeness": N, "coherence": N, "actionability": N, "overall": N, "comments": "简要评价"}`, query, answer, toolDataStr)

	resp, err := j.model.Generate(ctx, []*schema.Message{
		schema.UserMessage(prompt),
	})
	if err != nil {
		return nil, fmt.Errorf("judge LLM call failed: %w", err)
	}
	if resp == nil || resp.Content == "" {
		return nil, fmt.Errorf("judge LLM returned empty response")
	}

	scores, err := parseDiagScores(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse judge scores: %w (response: %s)", err, truncate(resp.Content, 200))
	}

	return scores, nil
}

// parseDiagScores extracts DiagScores from LLM JSON output.
func parseDiagScores(content string) (*DiagScores, error) {
	content = strings.TrimSpace(content)

	var scores DiagScores
	if err := json.Unmarshal([]byte(content), &scores); err == nil {
		return clampScores(&scores), nil
	}

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		jsonStr := content[start : end+1]
		if err := json.Unmarshal([]byte(jsonStr), &scores); err == nil {
			return clampScores(&scores), nil
		}
	}

	return nil, fmt.Errorf("no valid JSON found in response")
}

// clampScores ensures all scores are in [1, 5] range.
func clampScores(s *DiagScores) *DiagScores {
	s.Correctness = clamp(s.Correctness, 1, 5)
	s.Completeness = clamp(s.Completeness, 1, 5)
	s.Coherence = clamp(s.Coherence, 1, 5)
	s.Actionability = clamp(s.Actionability, 1, 5)
	s.Overall = clamp(s.Overall, 1, 5)
	return s
}

func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
