package experts

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	einoschema "github.com/cloudwego/eino/schema"
)

type evidenceAtom struct {
	SourceType      string
	SignalType      string
	Entity          string
	Title           string
	Snippet         string
	ArtifactRef     string
	ObservationTime time.Time
}

var (
	evidenceEntityPattern      = regexp.MustCompile(`\[([^\]]+)\]`)
	evidenceTimePattern        = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z`)
	evidenceWindowStartPattern = regexp.MustCompile(`(?m)^- (?:observation_window_utc|time_window_utc):\s*(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s*->`)
)

func atomizeToolEvidence(toolName, output, targetID string, observedAt time.Time, maxItems int) []EvidenceItem {
	content := strings.TrimSpace(redactSecrets(output))
	if content == "" {
		return nil
	}

	payload := content
	artifactRef := ""
	entity := ""
	observationTime := observedAt
	var wrapper map[string]any
	if json.Unmarshal([]byte(content), &wrapper) == nil {
		artifactRef = firstString(wrapper, "artifact_ref", "uri")
		entity = entityFromValue(wrapper)
		if parsed := timeFromValue(wrapper); !parsed.IsZero() {
			observationTime = parsed
		}
		if data, exists := wrapper["data"]; exists && data != nil {
			switch value := data.(type) {
			case string:
				if strings.TrimSpace(value) != "" {
					payload = value
				}
			default:
				if encoded, err := json.Marshal(value); err == nil {
					payload = string(encoded)
				}
			}
		}
	}

	defaultType := signalTypeForTool(toolName)
	atoms := atomizeEvidenceContent(payload, defaultType, entity, observationTime, artifactRef)
	atoms = limitEvidenceAtoms(atoms, maxItems)
	items := make([]EvidenceItem, 0, len(atoms))
	for _, atom := range atoms {
		identity := strings.Join([]string{atom.ArtifactRef, atom.Entity, atom.Snippet}, "\x1f")
		items = append(items, EvidenceItem{
			SourceType:         atom.SourceType,
			SourceID:           evidenceSourceID(toolName+":"+atom.SourceType, identity),
			SignalType:         atom.SignalType,
			Entity:             atom.Entity,
			Title:              atom.Title,
			Snippet:            atom.Snippet,
			Score:              1,
			Relation:           EvidenceRelationNeutral,
			TargetHypothesisID: targetID,
			ToolName:           toolName,
			ArtifactRef:        atom.ArtifactRef,
			ObservationTime:    atom.ObservationTime,
		})
	}
	return items
}

func atomizeRAGEvidence(docs []*einoschema.Document, targetID string, observedAt time.Time, maxItems int) []EvidenceItem {
	atoms := make([]evidenceAtom, 0, len(docs))
	for _, doc := range docs {
		if doc == nil || strings.TrimSpace(doc.Content) == "" {
			continue
		}
		artifactRef := metadataString(doc.MetaData, "artifact_ref", "uri")
		entity := metadataString(doc.MetaData, "entity", "service", "pod", "node", "instance")
		observationTime := metadataTime(doc.MetaData)
		if observationTime.IsZero() {
			observationTime = observedAt
		}
		docAtoms := atomizeEvidenceContent(redactSecrets(doc.Content), "rag", entity, observationTime, artifactRef)
		title := metadataString(doc.MetaData, "title", "file_name")
		for index := range docAtoms {
			if title != "" && docAtoms[index].SourceType == "rag" {
				docAtoms[index].Title = title
			}
			if docAtoms[index].ArtifactRef == "" {
				docAtoms[index].ArtifactRef = artifactRef
			}
			if docAtoms[index].Entity == "" {
				docAtoms[index].Entity = entity
			}
			if docAtoms[index].ObservationTime.IsZero() {
				docAtoms[index].ObservationTime = observationTime
			}
		}
		atoms = append(atoms, docAtoms...)
	}

	atoms = limitEvidenceAtoms(atoms, maxItems)
	items := make([]EvidenceItem, 0, len(atoms))
	for _, atom := range atoms {
		identity := strings.Join([]string{atom.ArtifactRef, atom.Entity, atom.Snippet}, "\x1f")
		items = append(items, EvidenceItem{
			SourceType:         atom.SourceType,
			SourceID:           evidenceSourceID("rag:"+atom.SourceType, identity),
			SignalType:         atom.SignalType,
			Entity:             atom.Entity,
			Title:              atom.Title,
			Snippet:            atom.Snippet,
			Score:              1,
			Relation:           EvidenceRelationNeutral,
			TargetHypothesisID: targetID,
			ArtifactRef:        atom.ArtifactRef,
			ObservationTime:    atom.ObservationTime,
		})
	}
	return items
}

func atomizeEvidenceContent(content, defaultType, defaultEntity string, defaultTime time.Time, artifactRef string) []evidenceAtom {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if parsed := evidenceWindowStart(content); !parsed.IsZero() {
		defaultTime = parsed
	}

	sectionType := ""
	atoms := make([]evidenceAtom, 0)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "## Metric Signals":
			sectionType = "metric"
			continue
		case "## Log Signals":
			sectionType = "log"
			continue
		case "## Trace Signals":
			sectionType = "trace"
			continue
		case "## Alert Signals":
			sectionType = "alert"
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			sectionType = ""
			continue
		}
		if sectionType == "" || !strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(strings.ToLower(trimmed), "- no ") {
			continue
		}
		snippet := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		if snippet == "" {
			continue
		}
		entity := entityFromText(snippet)
		if entity == "" {
			entity = defaultEntity
		}
		observedAt := timeFromText(snippet)
		if observedAt.IsZero() {
			observedAt = defaultTime
		}
		atoms = append(atoms, evidenceAtom{
			SourceType:      sectionType,
			SignalType:      sectionType,
			Entity:          entity,
			Title:           signalTitle(sectionType, entity),
			Snippet:         snippet,
			ArtifactRef:     artifactRef,
			ObservationTime: observedAt,
		})
	}
	if len(atoms) > 0 {
		return atoms
	}

	if structured := atomizeJSONContent(content, defaultType, defaultEntity, defaultTime, artifactRef); len(structured) > 0 {
		return structured
	}
	if defaultType == "" {
		defaultType = "evidence"
	}
	return []evidenceAtom{{
		SourceType:      defaultType,
		SignalType:      defaultType,
		Entity:          defaultEntity,
		Title:           signalTitle(defaultType, defaultEntity),
		Snippet:         content,
		ArtifactRef:     artifactRef,
		ObservationTime: defaultTime,
	}}
}

func atomizeJSONContent(content, defaultType, defaultEntity string, defaultTime time.Time, artifactRef string) []evidenceAtom {
	var payload any
	if json.Unmarshal([]byte(content), &payload) != nil {
		return nil
	}
	var values []any
	signalType := defaultType
	switch value := payload.(type) {
	case []any:
		values = value
	case map[string]any:
		for _, candidate := range []struct {
			key        string
			signalType string
		}{
			{key: "alerts", signalType: "alert"},
			{key: "samples", signalType: "metric"},
			{key: "evidence", signalType: defaultType},
			{key: "results", signalType: defaultType},
			{key: "documents", signalType: "rag"},
			{key: "logs", signalType: "log"},
		} {
			if items, ok := value[candidate.key].([]any); ok && len(items) > 0 {
				values = items
				signalType = candidate.signalType
				break
			}
		}
	}
	if len(values) == 0 {
		return nil
	}
	if signalType == "" {
		signalType = "evidence"
	}
	atoms := make([]evidenceAtom, 0, len(values))
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		entity := entityFromValue(value)
		if entity == "" {
			entity = defaultEntity
		}
		observedAt := timeFromValue(value)
		if observedAt.IsZero() {
			observedAt = defaultTime
		}
		atoms = append(atoms, evidenceAtom{
			SourceType:      signalType,
			SignalType:      signalType,
			Entity:          entity,
			Title:           signalTitle(signalType, entity),
			Snippet:         string(encoded),
			ArtifactRef:     artifactRef,
			ObservationTime: observedAt,
		})
	}
	return atoms
}

func limitEvidenceAtoms(atoms []evidenceAtom, maxItems int) []evidenceAtom {
	if maxItems <= 0 {
		maxItems = 1
	}
	if len(atoms) <= maxItems {
		return atoms
	}
	selected := make([]evidenceAtom, 0, maxItems)
	selectedIndex := make(map[int]struct{}, maxItems)
	for _, signalType := range []string{"metric", "log", "trace", "alert", "rag", "tool"} {
		for index, atom := range atoms {
			if atom.SignalType != signalType {
				continue
			}
			selected = append(selected, atom)
			selectedIndex[index] = struct{}{}
			break
		}
		if len(selected) >= maxItems {
			return selected
		}
	}
	for index, atom := range atoms {
		if _, exists := selectedIndex[index]; exists {
			continue
		}
		selected = append(selected, atom)
		if len(selected) >= maxItems {
			break
		}
	}
	return selected
}

func signalTypeForTool(toolName string) string {
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	switch {
	case strings.Contains(toolName, "alert"):
		return "alert"
	case strings.Contains(toolName, "prometheus") || strings.Contains(toolName, "metric"):
		return "metric"
	case strings.Contains(toolName, "log"):
		return "log"
	case strings.Contains(toolName, "doc"):
		return "rag"
	default:
		return "tool"
	}
}

func signalTitle(signalType, entity string) string {
	title := strings.TrimSpace(signalType) + " signal"
	if strings.TrimSpace(entity) != "" {
		title += " [" + strings.TrimSpace(entity) + "]"
	}
	return strings.TrimSpace(title)
}

func entityFromText(value string) string {
	if match := evidenceEntityPattern.FindStringSubmatch(value); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	if at := strings.Index(value, " @ "); at > 0 {
		return strings.TrimSpace(value[:at])
	}
	return ""
}

func entityFromValue(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if entity := firstString(object, "entity", "service", "pod", "node", "instance", "namespace"); entity != "" {
		return entity
	}
	if labels, ok := object["labels"].(map[string]any); ok {
		return firstString(labels, "service", "pod", "node", "instance", "namespace")
	}
	return ""
}

func timeFromText(value string) time.Time {
	raw := evidenceTimePattern.FindString(value)
	if raw == "" {
		return time.Time{}
	}
	parsed, _ := time.Parse(time.RFC3339Nano, raw)
	return parsed
}

func evidenceWindowStart(value string) time.Time {
	match := evidenceWindowStartPattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return time.Time{}
	}
	parsed, _ := time.Parse(time.RFC3339Nano, match[1])
	return parsed
}

func timeFromValue(value any) time.Time {
	object, ok := value.(map[string]any)
	if !ok {
		return time.Time{}
	}
	for _, key := range []string{"observation_time", "observed_at", "timestamp", "time", "start_time", "end_time"} {
		if raw, ok := object[key].(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw)); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func metadataString(metadata map[string]any, keys ...string) string {
	if metadata == nil {
		return ""
	}
	return firstString(metadata, keys...)
}

func metadataTime(metadata map[string]any) time.Time {
	if metadata == nil {
		return time.Time{}
	}
	return timeFromValue(metadata)
}

func formatEvidenceHistory(atoms []EvidenceItem, query, source string, maxChars int) []RetrievalRecord {
	records := make([]RetrievalRecord, 0, len(atoms))
	for _, atom := range atoms {
		output := atom.Snippet
		if maxChars > 0 {
			output = truncateString(output, maxChars)
		}
		payload, _ := json.Marshal(map[string]string{
			"signal_type":  atom.SignalType,
			"entity":       atom.Entity,
			"artifact_ref": atom.ArtifactRef,
			"data":         output,
		})
		records = append(records, RetrievalRecord{Query: query, Output: string(payload), Tool: source})
	}
	return records
}
