package contextcompression

import (
	"encoding/json"
	"strings"
)

// compressJSON 压缩 JSON 数组
// 策略: 保留首 N 项 + 尾 M 项 + 包含错误关键词/命中 query 的项
func compressJSON(content, query string, preserveFirst, preserveLast int) (string, bool) {
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(content), &items); err != nil {
		return "", false
	}

	total := len(items)
	if total == 0 {
		return content, true
	}

	// 如果项数不超过保留数量，不需要压缩
	if total <= preserveFirst+preserveLast {
		return content, true
	}

	queryLower := strings.ToLower(query)
	queryTerms := extractQueryTerms(queryLower)

	// 标记需要保留的项
	keep := make([]bool, total)

	// 1. 保留首部
	for i := 0; i < preserveFirst && i < total; i++ {
		keep[i] = true
	}

	// 2. 保留尾部
	for i := total - preserveLast; i < total; i++ {
		if i >= 0 {
			keep[i] = true
		}
	}

	// 3. 保留包含错误关键词或命中 query 的项
	for i, item := range items {
		if keep[i] {
			continue
		}
		itemStr := strings.ToLower(string(item))
		if containsAny(itemStr, errorKeywords) {
			keep[i] = true
			continue
		}
		for _, term := range queryTerms {
			if len(term) >= 2 && strings.Contains(itemStr, term) {
				keep[i] = true
				break
			}
		}
	}

	// 构建压缩结果
	result := make([]json.RawMessage, 0, total)
	for i, item := range items {
		if keep[i] {
			result = append(result, item)
		}
	}

	// 如果没有压缩（所有项都被保留），返回原内容
	if len(result) == total {
		return content, true
	}

	// 如果压缩后为空（不太可能），保留首项
	if len(result) == 0 {
		result = []json.RawMessage{items[0]}
	}

	compressed, err := json.Marshal(result)
	if err != nil {
		return "", false
	}
	return string(compressed), true
}

// extractQueryTerms 从 query 中提取搜索词
func extractQueryTerms(queryLower string) []string {
	// 按空格和常见分隔符分割
	seps := []string{" ", ",", ";", "|", "，", "；"}
	terms := []string{queryLower}
	for _, sep := range seps {
		var newTerms []string
		for _, t := range terms {
			parts := strings.Split(t, sep)
			newTerms = append(newTerms, parts...)
		}
		terms = newTerms
	}

	// 过滤太短的词
	result := make([]string, 0, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if len(t) >= 2 {
			result = append(result, t)
		}
	}
	return result
}
