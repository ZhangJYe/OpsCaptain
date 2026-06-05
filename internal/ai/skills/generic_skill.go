package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/protocol"
)

// MCPInvoker abstracts the MCP tool invocation so GenericSkill stays testable.
type MCPInvoker interface {
	Invoke(ctx context.Context, toolID, args string) (string, error)
}

// GenericSkill implements Skill and FocusProvider for user-defined skills.
// It calls an MCP tool and parses the output using configurable templates.
type GenericSkill struct {
	config  UserSkill
	invoker MCPInvoker
}

// NewGenericSkill creates a GenericSkill from a UserSkill config and an MCPInvoker.
func NewGenericSkill(config UserSkill, invoker MCPInvoker) *GenericSkill {
	return &GenericSkill{config: config, invoker: invoker}
}

func (s *GenericSkill) Name() string        { return s.config.Name }
func (s *GenericSkill) Description() string { return s.config.Description }
func (s *GenericSkill) Focus() string       { return s.config.Focus }

// Match returns true when the task goal contains any of the configured keywords.
func (s *GenericSkill) Match(task *protocol.TaskEnvelope) bool {
	if task == nil || len(s.config.Keywords) == 0 {
		return false
	}
	return ContainsAny(task.Goal, s.config.Keywords...)
}

// Run invokes the MCP tool and parses the output into a TaskResult.
// On invocation error a degraded result is returned (never a Go error).
func (s *GenericSkill) Run(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	// 1. Build input JSON
	inputJSON, _ := json.Marshal(map[string]string{"query": task.Goal})

	// 2. Invoke MCP tool
	output, invokeErr := s.invoker.Invoke(ctx, s.config.ToolRefID, string(inputJSON))

	// 3. Handle invoke error: return degraded result
	if invokeErr != nil {
		return &protocol.TaskResult{
			TaskID:            task.TaskID,
			Agent:             s.config.Name,
			Status:            protocol.ResultStatusDegraded,
			Summary:           fmt.Sprintf("MCP tool invocation failed: %s", invokeErr.Error()),
			Confidence:        0.25,
			DegradationReason: invokeErr.Error(),
			Metadata:          map[string]any{"tool_ref_id": s.config.ToolRefID},
		}, nil
	}

	// 4. Parse output based on OutputParser mode
	evidence := s.parseOutput(output)

	// 5. Build result with appropriate confidence
	status := protocol.ResultStatusSucceeded
	confidence := 0.70
	summary := fmt.Sprintf("Skill %q executed successfully", s.config.Name)

	if len(evidence) == 0 {
		confidence = 0.40
		summary = fmt.Sprintf("Skill %q returned no evidence", s.config.Name)
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      s.config.Name,
		Status:     status,
		Summary:    summary,
		Confidence: confidence,
		Evidence:   evidence,
		Metadata:   map[string]any{"tool_ref_id": s.config.ToolRefID},
	}, nil
}

// parseOutput dispatches to the appropriate parser based on OutputParser mode.
func (s *GenericSkill) parseOutput(output string) []protocol.EvidenceItem {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	switch s.config.OutputParser {
	case ParserJSONArray:
		return parseJSONArray(output)
	case ParserJSONNested:
		return parseJSONNested(output, s.config.JSONPath)
	case ParserLogLines:
		return parseLogLines(output)
	case ParserRaw:
		return parseRaw(output)
	default:
		return parseRaw(output)
	}
}

// parseJSONArray unmarshals a JSON array and extracts title/content per item.
func parseJSONArray(output string) []protocol.EvidenceItem {
	var items []map[string]any
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		return nil
	}
	evidence := make([]protocol.EvidenceItem, 0, len(items))
	for _, item := range items {
		title := extractString(item, "title", "name", "label")
		content := extractString(item, "content", "body", "text", "value", "description")
		snippet := content
		if title == "" && snippet == "" {
			continue
		}
		if title == "" {
			title = truncate(snippet, 80)
		}
		evidence = append(evidence, protocol.EvidenceItem{
			SourceType: "mcp_tool",
			Title:      title,
			Snippet:    snippet,
		})
	}
	return evidence
}

// parseJSONNested unmarshals a JSON object, traverses a dot-separated JSONPath,
// then parses the resulting value as a JSON array.
func parseJSONNested(output, jsonPath string) []protocol.EvidenceItem {
	var root map[string]any
	if err := json.Unmarshal([]byte(output), &root); err != nil {
		return nil
	}
	if jsonPath == "" {
		return nil
	}
	node := traverseJSONPath(root, jsonPath)
	if node == nil {
		return nil
	}
	// node should be a []any (JSON array)
	arrJSON, err := json.Marshal(node)
	if err != nil {
		return nil
	}
	return parseJSONArray(string(arrJSON))
}

// parseLogLines splits output by newlines, each non-empty line becomes one EvidenceItem.
func parseLogLines(output string) []protocol.EvidenceItem {
	lines := strings.Split(output, "\n")
	var evidence []protocol.EvidenceItem
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		evidence = append(evidence, protocol.EvidenceItem{
			SourceType: "log_line",
			Title:      fmt.Sprintf("Line %d", i+1),
			Snippet:    line,
		})
	}
	return evidence
}

// parseRaw treats the entire output as a single EvidenceItem.
func parseRaw(output string) []protocol.EvidenceItem {
	return []protocol.EvidenceItem{
		{
			SourceType: "mcp_tool",
			Title:      "Tool Output",
			Snippet:    output,
		},
	}
}

// extractString returns the first non-empty string value found under the given keys.
func extractString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

// traverseJSONPath walks a dot-separated path through a map structure.
func traverseJSONPath(root map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var current any = root
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}
	return current
}

// truncate shortens s to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen > 3 {
		return s[:maxLen-3] + "..."
	}
	return s[:maxLen]
}
