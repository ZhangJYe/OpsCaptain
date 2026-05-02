package events

import (
	"context"
	"testing"
)

func TestParseWarnings_NoOmission(t *testing.T) {
	warnings := parseWarnings("无遗漏")
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(warnings))
	}
}

func TestParseWarnings_Accurate(t *testing.T) {
	warnings := parseWarnings("全部准确")
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(warnings))
	}
}

func TestParseWarnings_Omissions(t *testing.T) {
	content := `[遗漏] payment_service 的错误率 5.2% 未被提及
[遗漏] order_service 的 P99 延迟 3200ms 未被提及`
	warnings := parseWarnings(content)
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(warnings))
	}
	if warnings[0] != "[遗漏] payment_service 的错误率 5.2% 未被提及" {
		t.Errorf("unexpected warning: %s", warnings[0])
	}
}

func TestParseWarnings_AccuracyIssues(t *testing.T) {
	content := `[问题] AI 称 P99 延迟为 2300ms，但工具数据显示 3200ms
[问题] AI 称无告警，但工具数据显示有 2 个 firing 告警`
	warnings := parseWarnings(content)
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(warnings))
	}
}

func TestParseWarnings_MixedContent(t *testing.T) {
	content := `根据分析结果：
[遗漏] 错误率 5.2% 未被提及
总结完毕`
	warnings := parseWarnings(content)
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
}

func TestParseWarnings_EmptyContent(t *testing.T) {
	warnings := parseWarnings("")
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(warnings))
	}
}

func TestLLMValidator_Disabled(t *testing.T) {
	v := NewLLMValidator(nil, &LLMValidationConfig{Enabled: false})
	result := v.Validate(nil, "test output", []string{"tool result"})
	if len(result.OmissionWarnings) != 0 || len(result.AccuracyWarnings) != 0 {
		t.Error("expected no warnings when disabled")
	}
}

func TestLLMValidator_NilConfig(t *testing.T) {
	v := NewLLMValidator(nil, nil)
	result := v.Validate(nil, "test output", []string{"tool result"})
	if len(result.OmissionWarnings) != 0 || len(result.AccuracyWarnings) != 0 {
		t.Error("expected no warnings with nil config")
	}
}

func TestLLMValidator_NoToolResults(t *testing.T) {
	v := NewLLMValidator(nil, &LLMValidationConfig{
		Enabled:           true,
		OmissionDetection: true,
		AccuracyCheck:     true,
	})
	result := v.Validate(context.Background(), "test output", nil)
	if len(result.OmissionWarnings) != 0 || len(result.AccuracyWarnings) != 0 {
		t.Error("expected no warnings with no tool results")
	}
}
