package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/models"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

const defaultJudgeTimeout = 30 * time.Second

type LLMJudge struct {
	Complete func(context.Context, string) (string, error)
	Timeout  time.Duration
}

func NewDeepSeekJudge() LLMJudge {
	return LLMJudge{Complete: completeWithDeepSeek}
}

func (j LLMJudge) Score(ctx context.Context, query, report string) (DiagScores, error) {
	if j.Complete == nil {
		return DiagScores{}, fmt.Errorf("judge completer is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := j.Timeout
	if timeout <= 0 {
		timeout = judgeTimeout(ctx)
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	raw, err := j.Complete(callCtx, BuildJudgePrompt(query, report))
	if err != nil {
		return DiagScores{}, err
	}
	return ParseDiagScores(raw)
}

func BuildJudgePrompt(query, report string) string {
	return fmt.Sprintf(`你是一个运维诊断质量评估专家。请对以下诊断报告从四个维度打分（1-5分）。

用户问题：
%s

诊断报告：
%s

评分维度：
1. 正确性：诊断结论是否与证据一致？有没有明显错误？
2. 完整性：是否覆盖了必要的排查维度？
3. 逻辑性：推理过程是否清晰、连贯？
4. 可操作性：建议是否具体可执行？

只输出 JSON，不要输出 Markdown 代码块或额外解释：
{
  "correctness": 4,
  "completeness": 3,
  "coherence": 4,
  "actionability": 5,
  "overall": 4,
  "comments": "一句话评价"
}`, strings.TrimSpace(query), strings.TrimSpace(report))
}

func ParseDiagScores(raw string) (DiagScores, error) {
	payload := extractJSONObject(raw)
	if payload == "" {
		return DiagScores{}, fmt.Errorf("judge response does not contain JSON")
	}
	var scores DiagScores
	if err := json.Unmarshal([]byte(payload), &scores); err != nil {
		return DiagScores{}, fmt.Errorf("decode judge scores: %w", err)
	}
	if scores.Overall == 0 {
		scores.Overall = roundedAverage(scores.Correctness, scores.Completeness, scores.Coherence, scores.Actionability)
	}
	if err := validateScores(scores); err != nil {
		return DiagScores{}, err
	}
	return scores, nil
}

func CompareRuns(ctx context.Context, cases []DiagCase, baselineRunner, candidateRunner Runner, judge Judge) ([]JudgeResult, error) {
	if judge == nil {
		return nil, fmt.Errorf("judge is nil")
	}
	report, err := RunAB(ctx, cases, baselineRunner, candidateRunner, judge)
	if err != nil {
		return nil, err
	}
	results := make([]JudgeResult, 0, len(report.Results))
	for _, item := range report.Results {
		results = append(results, JudgeResult{
			CaseID:          item.CaseID,
			Query:           item.Query,
			BaselineScores:  item.BaselineScores,
			CandidateScores: item.CandidateScores,
			Delta:           item.DeltaScores,
		})
	}
	return results, nil
}

func CalibrateJudge(ctx context.Context, cases []CalibrationCase, judge Judge) (float64, error) {
	if judge == nil {
		return 0, fmt.Errorf("judge is nil")
	}
	if len(cases) == 0 {
		return 0, nil
	}
	var total float64
	for _, item := range cases {
		score, err := judge.Score(ctx, item.Query, item.Report)
		if err != nil {
			return 0, fmt.Errorf("judge calibration case %s: %w", item.ID, err)
		}
		expected := item.HumanScore
		if expected == 0 {
			expected = item.ExpectedScores.Overall
		}
		if expected == 0 {
			return 0, fmt.Errorf("calibration case %s has no human score", item.ID)
		}
		total += math.Abs(float64(score.Overall - expected))
	}
	return total / float64(len(cases)), nil
}

func completeWithDeepSeek(ctx context.Context, prompt string) (string, error) {
	chatModel, err := models.OpenAIForDeepSeekV3Quick(ctx)
	if err != nil {
		return "", err
	}
	resp, err := chatModel.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: "你只输出合法 JSON。"},
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

func judgeTimeout(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "multi_agent.eval_judge_timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultJudgeTimeout
}

func extractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return ""
	}
	return raw[start : end+1]
}

func validateScores(scores DiagScores) error {
	for name, score := range map[string]int{
		"correctness":   scores.Correctness,
		"completeness":  scores.Completeness,
		"coherence":     scores.Coherence,
		"actionability": scores.Actionability,
		"overall":       scores.Overall,
	} {
		if score < 1 || score > 5 {
			return fmt.Errorf("judge score %s out of range: %d", name, score)
		}
	}
	return nil
}

func roundedAverage(values ...int) int {
	total := 0
	count := 0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		total += value
		count++
	}
	if count == 0 {
		return 0
	}
	return int(math.Round(float64(total) / float64(count)))
}

func scoreDelta(candidate, baseline DiagScores) DiagScores {
	return DiagScores{
		Correctness:   candidate.Correctness - baseline.Correctness,
		Completeness:  candidate.Completeness - baseline.Completeness,
		Coherence:     candidate.Coherence - baseline.Coherence,
		Actionability: candidate.Actionability - baseline.Actionability,
		Overall:       candidate.Overall - baseline.Overall,
	}
}
