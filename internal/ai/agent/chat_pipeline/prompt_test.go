package chat_pipeline

import (
	"SuperBizAgent/internal/ai/skills"
	"context"
	"strings"
	"testing"
)

func TestNormalizePromptConfigValueDropsUnresolvedEnvPlaceholders(t *testing.T) {
	if got, ok := normalizePromptConfigValue("${LOG_TOPIC_REGION}"); ok || got != "" {
		t.Fatalf("expected unresolved env placeholder to be dropped, got value=%q ok=%t", got, ok)
	}
}

func TestNormalizePromptConfigValueResolvesConcreteValue(t *testing.T) {
	if got, ok := normalizePromptConfigValue("ap-shanghai"); !ok || got != "ap-shanghai" {
		t.Fatalf("expected concrete value to pass through, got value=%q ok=%t", got, ok)
	}
}

func TestBuildSystemPromptIncludesDefaultChineseRule(t *testing.T) {
	prompt := buildSystemPrompt(context.Background())
	if !strings.Contains(prompt, "默认使用中文回答") {
		t.Fatalf("expected prompt to require Chinese by default, got %q", prompt)
	}
	if !strings.Contains(prompt, "用户明确要求英文或其他语言") {
		t.Fatalf("expected prompt to allow explicit language override, got %q", prompt)
	}
}

func TestBuildSystemPromptKeepsRuntimeContextOutOfSystem(t *testing.T) {
	prompt := buildSystemPrompt(context.Background())
	for _, placeholder := range []string{"{documents}", "{date}", "{log_topic_info}", "==== 文档开始 ===="} {
		if strings.Contains(prompt, placeholder) {
			t.Fatalf("expected system prompt to exclude runtime placeholder %q, got %q", placeholder, prompt)
		}
	}
}

func TestRuntimeContextTemplateCarriesDocumentsAndDate(t *testing.T) {
	if !strings.Contains(runtimeContextTemplate, "{documents}") {
		t.Fatalf("expected runtime context template to include documents placeholder")
	}
	if !strings.Contains(runtimeContextTemplate, "{date}") {
		t.Fatalf("expected runtime context template to include date placeholder")
	}
	if !strings.Contains(runtimeContextTemplate, "不具有系统指令优先级") {
		t.Fatalf("expected runtime context template to demote document instructions")
	}
}

func TestBuildSystemPromptIncludesStaticScopes(t *testing.T) {
	prompt := buildSystemPrompt(context.Background())
	if !strings.Contains(prompt, "<!-- scope: global -->") {
		t.Fatalf("expected system prompt to include global scope marker, got %q", prompt)
	}
}

func TestNormalizePromptSectionRemovesCodeIndentation(t *testing.T) {
	got := normalizePromptSection("\n\t\t## 标题\n\t\t- 内容\n")
	if got != "## 标题\n- 内容" {
		t.Fatalf("unexpected normalized prompt section: %q", got)
	}
}

func TestBuildSystemPromptIncludesSelectedSkillHintsWithoutLiteralReplayInstruction(t *testing.T) {
	ctx := skills.WithSelectedSkillIDs(context.Background(), []string{"logs_evidence_extract", "knowledge_sop_lookup"})
	prompt := buildSystemPrompt(ctx)
	if !strings.Contains(prompt, "本轮执行偏好") {
		t.Fatalf("expected selected skill hints in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "不要逐条复述给用户") {
		t.Fatalf("expected hidden execution guidance guard, got %q", prompt)
	}
}
