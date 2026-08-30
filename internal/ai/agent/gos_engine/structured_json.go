package gos_engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func decodeStrictJSONObject(raw string, target any) error {
	object, err := extractSingleJSONObject(raw)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewBufferString(object))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON object")
	}
	return nil
}

func extractSingleJSONObject(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	start := strings.IndexByte(raw, '{')
	if start < 0 {
		return "", fmt.Errorf("JSON object is required")
	}
	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(raw); index++ {
		char := raw[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				if strings.Contains(raw[index+1:], "{") {
					return "", fmt.Errorf("expected exactly one JSON object")
				}
				return raw[start : index+1], nil
			}
			if depth < 0 {
				return "", fmt.Errorf("invalid JSON object")
			}
		}
	}
	return "", fmt.Errorf("unterminated JSON object")
}

func renderStructuredPrompt(template string, values map[string]string) string {
	rendered := template
	for key, value := range values {
		rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", value)
	}
	return rendered
}
