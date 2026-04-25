package eval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadDiagCasesJSONL(path string) ([]DiagCase, error) {
	var cases []DiagCase
	if err := loadJSONL(path, func(raw []byte) error {
		var item DiagCase
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		cases = append(cases, item)
		return nil
	}); err != nil {
		return nil, err
	}
	return cases, nil
}

func WriteDiagCasesJSONL(path string, cases []DiagCase) error {
	return writeJSONL(path, cases, func(item DiagCase) string { return item.ID })
}

func LoadCalibrationCasesJSONL(path string) ([]CalibrationCase, error) {
	var cases []CalibrationCase
	if err := loadJSONL(path, func(raw []byte) error {
		var item CalibrationCase
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		cases = append(cases, item)
		return nil
	}); err != nil {
		return nil, err
	}
	return cases, nil
}

func WriteCalibrationCasesJSONL(path string, cases []CalibrationCase) error {
	return writeJSONL(path, cases, func(item CalibrationCase) string { return item.ID })
}

func loadJSONL(path string, decode func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open jsonl %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		if err := decode([]byte(raw)); err != nil {
			return fmt.Errorf("decode jsonl %s line %d: %w", path, lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan jsonl %s: %w", path, err)
	}
	return nil
}

func writeJSONL[T any](path string, items []T, id func(T) string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir jsonl dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create jsonl %s: %w", path, err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, item := range items {
		line, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("marshal jsonl item %s: %w", id(item), err)
		}
		if _, err := w.Write(line); err != nil {
			return fmt.Errorf("write jsonl item %s: %w", id(item), err)
		}
		if err := w.WriteByte('\n'); err != nil {
			return fmt.Errorf("terminate jsonl item %s: %w", id(item), err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush jsonl %s: %w", path, err)
	}
	return nil
}
