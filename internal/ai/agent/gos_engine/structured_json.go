package gos_engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func decodeStrictJSONObject(raw string, target any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON object")
	}
	return nil
}

func renderStructuredPrompt(template string, values map[string]string) string {
	rendered := template
	for key, value := range values {
		rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", value)
	}
	return rendered
}
