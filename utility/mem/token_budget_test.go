package mem

import (
	"strings"
	"testing"
)

func TestEstimateTokens_ChineseText(t *testing.T) {
	text := "你好世界"
	tokens := EstimateTokens(text)
	if tokens < 4 {
		t.Fatalf("expected >= 4 tokens for 4 Chinese chars, got %d", tokens)
	}
}

func TestEstimateTokens_EnglishText(t *testing.T) {
	text := "hello world this is a test"
	tokens := EstimateTokens(text)
	if tokens < 3 || tokens > 15 {
		t.Fatalf("expected 3-15 tokens for English text, got %d", tokens)
	}
}

func TestEstimateTokens_MixedText(t *testing.T) {
	text := "查看 order-service 的告警信息"
	tokens := EstimateTokens(text)
	if tokens <= 0 {
		t.Fatal("expected positive tokens for mixed text")
	}
}

func TestEstimateTokens_Empty(t *testing.T) {
	if EstimateTokens("") != 0 {
		t.Fatal("expected 0 tokens for empty string")
	}
}

func TestTrimToTokenBudget_NoTrim(t *testing.T) {
	text := "short text"
	result := TrimToTokenBudget(text, 1000)
	if result != text {
		t.Fatalf("expected no trim for short text, got '%s'", result)
	}
}

func TestTrimToTokenBudget_Trim(t *testing.T) {
	text := strings.Repeat("这是一段很长的中文文本。", 100)
	result := TrimToTokenBudget(text, 50)
	if len(result) >= len(text) {
		t.Fatal("expected text to be trimmed")
	}
	if !strings.HasSuffix(result, "...") {
		t.Fatal("expected trimmed text to end with ...")
	}
}

func TestTrimToTokenBudget_ZeroBudget(t *testing.T) {
	text := "some text"
	result := TrimToTokenBudget(text, 0)
	if result != text {
		t.Fatal("zero budget should not trim")
	}
}

func TestGetTokenBudget(t *testing.T) {
	budget := GetTokenBudget()
	if budget.MaxTokens <= 0 {
		t.Fatal("expected positive max tokens")
	}
	total := budget.SystemReserve + budget.MemoryReserve + budget.HistoryReserve + budget.DocumentReserve
	if total >= budget.MaxTokens {
		t.Fatalf("reserved tokens (%d) should be less than max (%d)", total, budget.MaxTokens)
	}
}

func TestEstimateTokens_LongChinese(t *testing.T) {
	text := strings.Repeat("你", 1000)
	tokens := EstimateTokens(text)
	if tokens < 1000 {
		t.Fatalf("expected >= 1000 tokens for 1000 Chinese chars, got %d", tokens)
	}
}

func TestEstimateTokens_LongEnglish(t *testing.T) {
	text := strings.Repeat("word ", 1000)
	tokens := EstimateTokens(text)
	if tokens < 500 || tokens > 2000 {
		t.Fatalf("expected 500-2000 tokens for 1000 English words, got %d", tokens)
	}
}
