package memory

import (
	"encoding/json"
	"fmt"
	"strings"
)

func memoryAgentSystemPrompt() string {
	return strings.Join([]string{
		"你是 OpsCaption 的 Memory Agent，只负责决定哪些对话内容应该进入长期记忆。",
		"你必须只输出 JSON，不要输出 Markdown、解释或多余文本。",
		"输出格式：{\"actions\":[{\"op\":\"skip|upsert|supersede|promote\",\"target_id\":\"\",\"type\":\"fact|preference|procedure|episode\",\"content\":\"\",\"scope\":\"session|user|project|global\",\"scope_id\":\"\",\"confidence\":0.0,\"conflict_group\":\"\",\"expires_at\":0,\"reason\":\"\"}]}",
		"只保存长期稳定、有复用价值的信息：用户偏好、项目约定、服务事实、排障流程、被明确纠正的新事实。",
		"不要保存临时闲聊、模型套话、代码块、密钥、token、password、authorization、一次性中间推理。",
		"用户个人偏好用 user scope；当前会话事实用 session scope；项目约定和排障流程用 project scope；global scope 只用于明确跨项目通用的稳定规则。",
		"如果新事实纠正了已有记忆，用 supersede 并填写 target_id；如果已有记忆应提升范围，用 promote；没有可保存内容就只返回 skip。",
		"content 必须简短、可直接复用，confidence 必须在 0 到 1 之间。",
	}, "\n")
}

func memoryAgentUserPrompt(event MemoryEvent) string {
	payload := struct {
		SessionID string               `json:"session_id"`
		UserID    string               `json:"user_id,omitempty"`
		ProjectID string               `json:"project_id,omitempty"`
		Query     string               `json:"query"`
		Answer    string               `json:"answer"`
		Existing  []memoryPromptMemory `json:"existing_memories,omitempty"`
	}{
		SessionID: event.SessionID,
		UserID:    event.UserID,
		ProjectID: event.ProjectID,
		Query:     event.Query,
		Answer:    event.Answer,
		Existing:  memoryPromptMemories(event.ExistingMemories),
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf("session_id=%s\nquery=%s\nanswer=%s", event.SessionID, event.Query, event.Answer)
	}
	return string(body)
}

type memoryPromptMemory struct {
	ID            string      `json:"id"`
	Type          MemoryType  `json:"type"`
	Content       string      `json:"content"`
	Scope         MemoryScope `json:"scope"`
	ScopeID       string      `json:"scope_id,omitempty"`
	Confidence    float64     `json:"confidence"`
	ConflictGroup string      `json:"conflict_group,omitempty"`
}

func memoryPromptMemories(entries []*MemoryEntry) []memoryPromptMemory {
	if len(entries) == 0 {
		return nil
	}
	result := make([]memoryPromptMemory, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		result = append(result, memoryPromptMemory{
			ID:            entry.ID,
			Type:          entry.Type,
			Content:       entry.Content,
			Scope:         entry.Scope,
			ScopeID:       entry.ScopeID,
			Confidence:    entry.Confidence,
			ConflictGroup: entry.ConflictGroup,
		})
	}
	return result
}

func parseMemoryDecisionJSON(content string) (*MemoryDecision, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, fmt.Errorf("empty memory agent response")
	}
	trimmed = stripJSONFence(trimmed)
	if start, end := strings.Index(trimmed, "{"), strings.LastIndex(trimmed, "}"); start >= 0 && end > start {
		body := trimmed[start : end+1]
		var decision MemoryDecision
		if err := json.Unmarshal([]byte(body), &decision); err == nil {
			if len(decision.Actions) > 0 {
				return &decision, nil
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal([]byte(body), &raw); err == nil {
				if _, ok := raw["actions"]; ok {
					return &decision, nil
				}
			}
			var action MemoryAction
			if err := json.Unmarshal([]byte(body), &action); err == nil && action.Op != "" {
				return &MemoryDecision{Actions: []MemoryAction{action}}, nil
			}
		}
	}
	if start, end := strings.Index(trimmed, "["), strings.LastIndex(trimmed, "]"); start >= 0 && end > start {
		body := trimmed[start : end+1]
		var actions []MemoryAction
		if err := json.Unmarshal([]byte(body), &actions); err == nil {
			return &MemoryDecision{Actions: actions}, nil
		}
	}
	return nil, fmt.Errorf("invalid memory agent json")
}

func stripJSONFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) <= 2 {
		return trimmed
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}