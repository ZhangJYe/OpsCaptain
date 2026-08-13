package loader

import (
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/schema"
)

type MarkdownFields struct {
	DocumentID string
	Title      string
	Provider   string
	Tags       []string
	Source     string
}

func ParseMarkdownFields(path string, raw []byte) MarkdownFields {
	name := filepath.Base(path)
	docID := strings.TrimSuffix(name, filepath.Ext(name))
	fields := MarkdownFields{DocumentID: docID, Title: docID, Source: path}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if fields.Title == docID && strings.HasPrefix(trimmed, "# ") {
			if title := strings.TrimSpace(strings.TrimPrefix(trimmed, "# ")); title != "" {
				fields.Title = title
			}
		}
		key, value, ok := markdownListField(trimmed)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "provider":
			fields.Provider = value
		case "tags":
			for _, tag := range strings.Split(value, ",") {
				if tag = strings.TrimSpace(tag); tag != "" {
					fields.Tags = append(fields.Tags, tag)
				}
			}
		case "source":
			fields.Source = value
		}
	}
	return fields
}

func EnrichMarkdownDocument(path string, raw []byte, doc *schema.Document) {
	if doc == nil || !strings.EqualFold(filepath.Ext(path), ".md") {
		return
	}
	fields := ParseMarkdownFields(path, raw)
	if strings.TrimSpace(doc.ID) == "" {
		doc.ID = fields.DocumentID
	}
	if doc.MetaData == nil {
		doc.MetaData = make(map[string]any)
	}
	setMetadataIfEmpty(doc.MetaData, "knowledge_doc_id", fields.DocumentID)
	setMetadataIfEmpty(doc.MetaData, "title", fields.Title)
	setMetadataIfEmpty(doc.MetaData, "provider", fields.Provider)
	if _, exists := doc.MetaData["tags"]; !exists && len(fields.Tags) > 0 {
		doc.MetaData["tags"] = append([]string(nil), fields.Tags...)
	}
	setMetadataIfEmpty(doc.MetaData, "source_uri", fields.Source)
	setMetadataIfEmpty(doc.MetaData, "_source", path)
}

func markdownListField(line string) (string, string, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	key, value = strings.TrimSpace(key), strings.TrimSpace(value)
	return key, value, key != "" && value != ""
}

func setMetadataIfEmpty(meta map[string]any, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if current, ok := meta[key].(string); ok && strings.TrimSpace(current) != "" {
		return
	}
	meta[key] = value
}
