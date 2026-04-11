package eval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LoadEvalCasesJSONL(path string) ([]EvalCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open eval cases %s: %w", path, err)
	}
	defer f.Close()

	var cases []EvalCase
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var item EvalCase
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("decode eval case line %d: %w", lineNo, err)
		}
		cases = append(cases, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan eval cases %s: %w", path, err)
	}
	return cases, nil
}

func WriteEvalCasesJSONL(path string, cases []EvalCase) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir eval dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create eval cases %s: %w", path, err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, item := range cases {
		line, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("marshal eval case %s: %w", item.ID, err)
		}
		if _, err := w.Write(line); err != nil {
			return fmt.Errorf("write eval case %s: %w", item.ID, err)
		}
		if err := w.WriteByte('\n'); err != nil {
			return fmt.Errorf("terminate eval case %s: %w", item.ID, err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush eval cases %s: %w", path, err)
	}
	return nil
}
