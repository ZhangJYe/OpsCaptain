package eval

import (
	"context"
	"strings"
	"testing"
)

func TestParseDiagScoresAcceptsFencedJSON(t *testing.T) {
	raw := "```json\n{\"correctness\":4,\"completeness\":3,\"coherence\":4,\"actionability\":5,\"comments\":\"ok\"}\n```"
	scores, err := ParseDiagScores(raw)
	if err != nil {
		t.Fatalf("parse scores: %v", err)
	}
	if scores.Overall != 4 {
		t.Fatalf("expected computed overall 4, got %#v", scores)
	}
}

func TestParseDiagScoresRejectsOutOfRange(t *testing.T) {
	_, err := ParseDiagScores(`{"correctness":6,"completeness":3,"coherence":4,"actionability":5,"overall":4}`)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out of range error, got %v", err)
	}
}

func TestLLMJudgeUsesPromptAndParser(t *testing.T) {
	judge := LLMJudge{
		Complete: func(_ context.Context, prompt string) (string, error) {
			if !strings.Contains(prompt, "paymentservice") || !strings.Contains(prompt, "诊断报告") {
				t.Fatalf("unexpected prompt: %s", prompt)
			}
			return `{"correctness":5,"completeness":4,"coherence":4,"actionability":5,"overall":5,"comments":"可用"}`, nil
		},
	}
	scores, err := judge.Score(context.Background(), "paymentservice CPU 高", "检查 resource limits 或 HPA 配置")
	if err != nil {
		t.Fatalf("score: %v", err)
	}
	if scores.Overall != 5 || scores.Actionability != 5 {
		t.Fatalf("unexpected scores: %#v", scores)
	}
}

func TestCalibrateJudge(t *testing.T) {
	cases, err := LoadCalibrationCasesJSONL("testdata/diag_calibration.jsonl")
	if err != nil {
		t.Fatalf("load calibration cases: %v", err)
	}
	judge := JudgeFunc(func(_ context.Context, _ string, report string) (DiagScores, error) {
		if strings.Contains(report, "只说系统可能异常") {
			return DiagScores{Correctness: 2, Completeness: 1, Coherence: 2, Actionability: 1, Overall: 2}, nil
		}
		return DiagScores{Correctness: 5, Completeness: 5, Coherence: 5, Actionability: 5, Overall: 5}, nil
	})

	avgDiff, err := CalibrateJudge(context.Background(), cases, judge)
	if err != nil {
		t.Fatalf("calibrate judge: %v", err)
	}
	if avgDiff != 0 {
		t.Fatalf("expected zero diff, got %.2f", avgDiff)
	}
}
