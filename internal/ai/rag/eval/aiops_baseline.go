package eval

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DefaultAIOPSEvalRatio    = 0.2
	MaxSymptomRelatedCaseIDs = 20
)

type AIOPSPrepOptions struct {
	DatasetRoot string
	OutputRoot  string
	EvalRatio   float64
}

type AIOPSPrepSummary struct {
	Cases                    int    `json:"cases"`
	EvidenceDocs             int    `json:"evidence_docs"`
	HistoryDocs              int    `json:"history_docs"`
	BuildEvidenceDocs        int    `json:"build_evidence_docs"`
	BuildHistoryDocs         int    `json:"build_history_docs"`
	EvalCases                int    `json:"eval_cases"`
	HoldoutEvalCases         int    `json:"holdout_eval_cases"`
	HoldoutRelatedEvalCases  int    `json:"holdout_related_eval_cases"`
	HoldoutSymptomEvalCases  int    `json:"holdout_symptom_eval_cases"`
	HoldoutCombinedEvalCases int    `json:"holdout_combined_eval_cases"`
	BuildCases               int    `json:"build_cases"`
	HoldoutCases             int    `json:"holdout_cases"`
	OutputRoot               string `json:"output_root"`
}

type AIOPSInputCase struct {
	UUID               string `json:"uuid"`
	AnomalyDescription string `json:"Anomaly Description"`
}

type AIOPSGroundTruth struct {
	FaultCategory    string                `json:"fault_category"`
	FaultType        string                `json:"fault_type"`
	InstanceType     string                `json:"instance_type"`
	Service          string                `json:"service"`
	Instance         StringOrList          `json:"instance"`
	Source           string                `json:"source"`
	Destination      string                `json:"destination"`
	StartTime        string                `json:"start_time"`
	EndTime          string                `json:"end_time"`
	UUID             string                `json:"uuid"`
	KeyObservations  []AIOPSKeyObservation `json:"key_observations"`
	KeyMetrics       []string              `json:"key_metrics"`
	FaultDescription []string              `json:"fault_description"`
}

type AIOPSKeyObservation struct {
	Type    string   `json:"type"`
	Keyword []string `json:"keyword"`
}

type AIOPSSplitManifest struct {
	Dataset        string    `json:"dataset"`
	GeneratedAt    time.Time `json:"generated_at"`
	TotalCases     int       `json:"total_cases"`
	EvalRatio      float64   `json:"eval_ratio"`
	BuildCaseIDs   []string  `json:"build_case_ids"`
	HoldoutCaseIDs []string  `json:"holdout_case_ids"`
}

type StringOrList []string

func (s *StringOrList) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*s = nil
		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = uniqueNonEmpty([]string{single})
		return nil
	}

	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*s = uniqueNonEmpty(list)
		return nil
	}
	return fmt.Errorf("unsupported string-or-list payload: %s", string(data))
}

func (s StringOrList) First() string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

func (s StringOrList) Joined(sep string) string {
	return strings.Join(uniqueNonEmpty([]string(s)), sep)
}

func GenerateAIOPSBaselineArtifacts(ctx context.Context, opts AIOPSPrepOptions) (AIOPSPrepSummary, error) {
	datasetRoot := strings.TrimSpace(opts.DatasetRoot)
	if datasetRoot == "" {
		datasetRoot = filepath.Join(".", "aiopschallenge2025")
	}
	outputRoot := strings.TrimSpace(opts.OutputRoot)
	if outputRoot == "" {
		outputRoot = filepath.Join(datasetRoot, "baseline")
	}
	evalRatio := opts.EvalRatio
	if evalRatio <= 0 || evalRatio >= 1 {
		evalRatio = DefaultAIOPSEvalRatio
	}

	inputs, err := loadAIOPSInput(filepath.Join(datasetRoot, "input.json"))
	if err != nil {
		return AIOPSPrepSummary{}, err
	}
	groundtruth, err := loadAIOPSGroundTruth(filepath.Join(datasetRoot, "groundtruth.jsonl"))
	if err != nil {
		return AIOPSPrepSummary{}, err
	}

	ids := collectOrderedCaseIDs(inputs, groundtruth)
	if len(ids) == 0 {
		return AIOPSPrepSummary{}, fmt.Errorf("no cases found under %s", datasetRoot)
	}

	docsEvidenceDir := filepath.Join(outputRoot, "docs_evidence")
	docsHistoryDir := filepath.Join(outputRoot, "docs_history")
	docsEvidenceBuildDir := filepath.Join(outputRoot, "docs_evidence_build")
	docsHistoryBuildDir := filepath.Join(outputRoot, "docs_history_build")
	evalDir := filepath.Join(outputRoot, "eval")
	for _, dir := range []string{docsEvidenceDir, docsHistoryDir, docsEvidenceBuildDir, docsHistoryBuildDir, evalDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return AIOPSPrepSummary{}, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	buildIDs, holdoutIDs := splitCaseIDs(ids, evalRatio)
	buildSet := make(map[string]struct{}, len(buildIDs))
	for _, id := range buildIDs {
		buildSet[id] = struct{}{}
	}

	allEvalCases := make([]EvalCase, 0, len(ids)*2)
	for _, id := range ids {
		inputCase, ok := inputs[id]
		if !ok {
			return AIOPSPrepSummary{}, fmt.Errorf("missing input record for case %s", id)
		}
		gt, ok := groundtruth[id]
		if !ok {
			return AIOPSPrepSummary{}, fmt.Errorf("missing groundtruth record for case %s", id)
		}

		evidenceDoc := renderEvidenceDoc(inputCase, gt)
		historyDoc := renderHistoryDoc(inputCase, gt)

		if err := os.WriteFile(filepath.Join(docsEvidenceDir, id+".md"), []byte(evidenceDoc), 0o644); err != nil {
			return AIOPSPrepSummary{}, fmt.Errorf("write evidence doc %s: %w", id, err)
		}
		if err := os.WriteFile(filepath.Join(docsHistoryDir, id+".md"), []byte(historyDoc), 0o644); err != nil {
			return AIOPSPrepSummary{}, fmt.Errorf("write history doc %s: %w", id, err)
		}
		if _, ok := buildSet[id]; ok {
			if err := os.WriteFile(filepath.Join(docsEvidenceBuildDir, id+".md"), []byte(evidenceDoc), 0o644); err != nil {
				return AIOPSPrepSummary{}, fmt.Errorf("write build evidence doc %s: %w", id, err)
			}
			if err := os.WriteFile(filepath.Join(docsHistoryBuildDir, id+".md"), []byte(historyDoc), 0o644); err != nil {
				return AIOPSPrepSummary{}, fmt.Errorf("write build history doc %s: %w", id, err)
			}
		}
		allEvalCases = append(allEvalCases, buildEvalCases(inputCase, gt)...)
	}

	holdoutSet := make(map[string]struct{}, len(holdoutIDs))
	for _, id := range holdoutIDs {
		holdoutSet[id] = struct{}{}
	}

	holdoutEvalCases := make([]EvalCase, 0, len(allEvalCases))
	holdoutRelatedEvalCases := make([]EvalCase, 0, len(allEvalCases))
	holdoutSymptomEvalCases := make([]EvalCase, 0, len(allEvalCases))
	holdoutCombinedEvalCases := make([]EvalCase, 0, len(allEvalCases))
	for _, item := range allEvalCases {
		if hasRelevantID(item, holdoutSet) {
			holdoutEvalCases = append(holdoutEvalCases, item)
			faultIDs := relatedBuildCaseIDs(item, buildIDs, groundtruth)
			if len(faultIDs) > 0 {
				related := item
				related.RelevantIDs = faultIDs
				related.Notes = appendEvalNotes(item.Notes, "relevant_ids derived from build split fault_type/fault_category matches")
				holdoutRelatedEvalCases = append(holdoutRelatedEvalCases, related)
			}

			symptomIDs := relatedBuildCaseIDsBySymptom(item, buildIDs, groundtruth)
			if len(symptomIDs) > 0 {
				symptom := item
				symptom.RelevantIDs = symptomIDs
				symptom.Notes = appendEvalNotes(item.Notes, "relevant_ids derived from build split symptom matches")
				holdoutSymptomEvalCases = append(holdoutSymptomEvalCases, symptom)
			}

			if combinedIDs := unionIDs(faultIDs, symptomIDs); len(combinedIDs) > 0 {
				combined := item
				combined.RelevantIDs = combinedIDs
				combined.Notes = appendEvalNotes(item.Notes, "relevant_ids derived from combined fault_type/fault_category and symptom matches")
				holdoutCombinedEvalCases = append(holdoutCombinedEvalCases, combined)
			}
		}
	}

	if err := WriteEvalCasesJSONL(filepath.Join(evalDir, "eval_cases.jsonl"), allEvalCases); err != nil {
		return AIOPSPrepSummary{}, err
	}
	if err := WriteEvalCasesJSONL(filepath.Join(evalDir, "eval_cases_holdout.jsonl"), holdoutEvalCases); err != nil {
		return AIOPSPrepSummary{}, err
	}
	if err := WriteEvalCasesJSONL(filepath.Join(evalDir, "eval_cases_holdout_related.jsonl"), holdoutRelatedEvalCases); err != nil {
		return AIOPSPrepSummary{}, err
	}
	if err := WriteEvalCasesJSONL(filepath.Join(evalDir, "eval_cases_holdout_symptom.jsonl"), holdoutSymptomEvalCases); err != nil {
		return AIOPSPrepSummary{}, err
	}
	if err := WriteEvalCasesJSONL(filepath.Join(evalDir, "eval_cases_holdout_combined.jsonl"), holdoutCombinedEvalCases); err != nil {
		return AIOPSPrepSummary{}, err
	}

	split := AIOPSSplitManifest{
		Dataset:        "aiopschallenge2025",
		GeneratedAt:    time.Now().UTC(),
		TotalCases:     len(ids),
		EvalRatio:      evalRatio,
		BuildCaseIDs:   buildIDs,
		HoldoutCaseIDs: holdoutIDs,
	}
	if err := writeJSON(filepath.Join(evalDir, "build_split.json"), split); err != nil {
		return AIOPSPrepSummary{}, err
	}
	if err := writeJSON(filepath.Join(evalDir, "eval_split.json"), split); err != nil {
		return AIOPSPrepSummary{}, err
	}

	_ = ctx
	return AIOPSPrepSummary{
		Cases:                    len(ids),
		EvidenceDocs:             len(ids),
		HistoryDocs:              len(ids),
		BuildEvidenceDocs:        len(buildIDs),
		BuildHistoryDocs:         len(buildIDs),
		EvalCases:                len(allEvalCases),
		HoldoutEvalCases:         len(holdoutEvalCases),
		HoldoutRelatedEvalCases:  len(holdoutRelatedEvalCases),
		HoldoutSymptomEvalCases:  len(holdoutSymptomEvalCases),
		HoldoutCombinedEvalCases: len(holdoutCombinedEvalCases),
		BuildCases:               len(buildIDs),
		HoldoutCases:             len(holdoutIDs),
		OutputRoot:               outputRoot,
	}, nil
}

func loadAIOPSInput(path string) (map[string]AIOPSInputCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read input json %s: %w", path, err)
	}
	var items []AIOPSInputCase
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("decode input json %s: %w", path, err)
	}
	out := make(map[string]AIOPSInputCase, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.UUID) == "" {
			continue
		}
		out[item.UUID] = item
	}
	return out, nil
}

func loadAIOPSGroundTruth(path string) (map[string]AIOPSGroundTruth, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open groundtruth %s: %w", path, err)
	}
	defer f.Close()

	out := make(map[string]AIOPSGroundTruth)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var item AIOPSGroundTruth
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, fmt.Errorf("decode groundtruth line %d: %w", lineNo, err)
		}
		if strings.TrimSpace(item.UUID) == "" {
			continue
		}
		out[item.UUID] = item
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan groundtruth %s: %w", path, err)
	}
	return out, nil
}

func collectOrderedCaseIDs(inputs map[string]AIOPSInputCase, groundtruth map[string]AIOPSGroundTruth) []string {
	idSet := make(map[string]struct{}, len(inputs))
	for id := range inputs {
		if _, ok := groundtruth[id]; ok {
			idSet[id] = struct{}{}
		}
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func renderEvidenceDoc(inputCase AIOPSInputCase, gt AIOPSGroundTruth) string {
	var b strings.Builder
	b.WriteString("# RCA 观测案例\n\n")
	fmt.Fprintf(&b, "- case_id: %s\n", gt.UUID)
	fmt.Fprintf(&b, "- service: %s\n", fallback(gt.Service, gt.Instance.First()))
	fmt.Fprintf(&b, "- instance_type: %s\n", gt.InstanceType)
	fmt.Fprintf(&b, "- instance: %s\n", nonEmptyOr(gt.Instance.Joined(", "), "unknown"))
	fmt.Fprintf(&b, "- start_time: %s\n", gt.StartTime)
	fmt.Fprintf(&b, "- end_time: %s\n", gt.EndTime)
	if strings.TrimSpace(gt.Source) != "" || strings.TrimSpace(gt.Destination) != "" {
		fmt.Fprintf(&b, "- network_path: %s -> %s\n", fallback(gt.Source, "unknown"), fallback(gt.Destination, "unknown"))
	}
	b.WriteString("\n## 异常描述\n\n")
	b.WriteString(nonEmptyOr(inputCase.AnomalyDescription, "系统出现异常，需要根据观测信息推断原因。"))
	b.WriteString("\n\n## 关键观测\n\n")
	for _, obs := range gt.KeyObservations {
		keywords := joinKeywords(obs.Keyword)
		if keywords == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s: %s\n", nonEmptyOr(obs.Type, "unknown"), keywords)
	}
	b.WriteString("\n## 检索关键词\n\n")
	keywords := observationKeywords(gt.KeyObservations)
	if service := strings.TrimSpace(fallback(gt.Service, gt.Instance.First())); service != "" {
		keywords = append([]string{service, gt.InstanceType}, keywords...)
	}
	if gt.Source != "" || gt.Destination != "" {
		keywords = append(keywords, gt.Source, gt.Destination)
	}
	fmt.Fprintf(&b, "%s\n", strings.Join(uniqueNonEmpty(keywords), " "))
	return b.String()
}

func renderHistoryDoc(inputCase AIOPSInputCase, gt AIOPSGroundTruth) string {
	var b strings.Builder
	b.WriteString(renderEvidenceDoc(inputCase, gt))
	b.WriteString("\n## 历史案例标签（非实时证据）\n\n")
	fmt.Fprintf(&b, "- fault_category: %s\n", gt.FaultCategory)
	fmt.Fprintf(&b, "- fault_type: %s\n", gt.FaultType)
	if len(gt.KeyMetrics) > 0 {
		fmt.Fprintf(&b, "- historical_key_metrics: %s\n", strings.Join(uniqueNonEmpty(gt.KeyMetrics), ", "))
	}
	if len(gt.FaultDescription) > 0 {
		b.WriteString("\n## 历史结案摘要（非实时证据）\n\n")
		for _, item := range uniqueNonEmpty(gt.FaultDescription) {
			fmt.Fprintf(&b, "- %s\n", item)
		}
	}
	return b.String()
}

func buildEvalCases(inputCase AIOPSInputCase, gt AIOPSGroundTruth) []EvalCase {
	service := fallback(gt.Service, gt.Instance.First())
	keywords := observationKeywords(gt.KeyObservations)
	if len(keywords) == 0 {
		keywords = []string{gt.InstanceType, "异常"}
	}
	topKeywords := joinTopKeywords(keywords, 4)
	queryBase := strings.TrimSpace(strings.Join(uniqueNonEmpty([]string{service, gt.InstanceType, topKeywords}), " "))
	if queryBase == "" {
		queryBase = gt.UUID
	}

	cases := []EvalCase{
		{
			ID:          gt.UUID + "-obs",
			Query:       strings.TrimSpace(queryBase + " 异常 排查"),
			RelevantIDs: []string{gt.UUID},
			Notes:       "evidence-oriented baseline query built from service and key observations",
		},
	}
	if strings.TrimSpace(gt.Source) != "" || strings.TrimSpace(gt.Destination) != "" {
		cases = append(cases, EvalCase{
			ID:          gt.UUID + "-path",
			Query:       strings.TrimSpace(strings.Join(uniqueNonEmpty([]string{gt.Source, gt.Destination, service, topKeywords, "调用异常"}), " ")),
			RelevantIDs: []string{gt.UUID},
			Notes:       "network path baseline query built from source/destination and observations",
		})
	} else {
		cases = append(cases, EvalCase{
			ID:          gt.UUID + "-symptom",
			Query:       strings.TrimSpace(strings.Join(uniqueNonEmpty([]string{service, topKeywords, "故障分析"}), " ")),
			RelevantIDs: []string{gt.UUID},
			Notes:       "symptom baseline query built from observations",
		})
	}
	_ = inputCase
	return cases
}

func splitCaseIDs(ids []string, evalRatio float64) ([]string, []string) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) == 1 {
		return append([]string(nil), ids...), nil
	}
	holdoutCount := int(math.Round(float64(len(ids)) * evalRatio))
	if holdoutCount < 1 {
		holdoutCount = 1
	}
	if holdoutCount >= len(ids) {
		holdoutCount = len(ids) - 1
	}
	buildCount := len(ids) - holdoutCount
	buildIDs := append([]string(nil), ids[:buildCount]...)
	holdoutIDs := append([]string(nil), ids[buildCount:]...)
	return buildIDs, holdoutIDs
}

func hasRelevantID(item EvalCase, allowed map[string]struct{}) bool {
	for _, id := range item.RelevantIDs {
		if _, ok := allowed[id]; ok {
			return true
		}
	}
	return false
}

func relatedBuildCaseIDs(item EvalCase, buildIDs []string, groundtruth map[string]AIOPSGroundTruth) []string {
	if len(item.RelevantIDs) == 0 {
		return nil
	}

	current, ok := groundtruth[item.RelevantIDs[0]]
	if !ok {
		return nil
	}

	currentType := strings.TrimSpace(current.FaultType)
	currentCategory := strings.TrimSpace(current.FaultCategory)
	typeMatches := make([]string, 0)
	categoryMatches := make([]string, 0)

	for _, id := range buildIDs {
		candidate, ok := groundtruth[id]
		if !ok {
			continue
		}
		if currentType != "" && strings.EqualFold(strings.TrimSpace(candidate.FaultType), currentType) {
			typeMatches = append(typeMatches, id)
			continue
		}
		if currentCategory != "" && strings.EqualFold(strings.TrimSpace(candidate.FaultCategory), currentCategory) {
			categoryMatches = append(categoryMatches, id)
		}
	}

	if len(typeMatches) > 0 {
		return uniqueNonEmpty(typeMatches)
	}
	return uniqueNonEmpty(categoryMatches)
}

func relatedBuildCaseIDsBySymptom(item EvalCase, buildIDs []string, groundtruth map[string]AIOPSGroundTruth) []string {
	if len(item.RelevantIDs) == 0 {
		return nil
	}
	current, ok := groundtruth[item.RelevantIDs[0]]
	if !ok {
		return nil
	}

	currentService := strings.ToLower(strings.TrimSpace(fallback(current.Service, current.Instance.First())))
	currentInstanceType := strings.ToLower(strings.TrimSpace(current.InstanceType))
	currentKeywords := observationKeywordSet(current.KeyObservations)
	currentSource := strings.ToLower(strings.TrimSpace(current.Source))
	currentDestination := strings.ToLower(strings.TrimSpace(current.Destination))

	type scored struct {
		id    string
		score int
	}
	var candidates []scored

	for _, id := range buildIDs {
		candidate, ok := groundtruth[id]
		if !ok {
			continue
		}
		s := 0
		candidateService := strings.ToLower(strings.TrimSpace(fallback(candidate.Service, candidate.Instance.First())))
		exactServiceMatch := currentService != "" && candidateService == currentService
		if exactServiceMatch {
			s += 3
		}
		if currentInstanceType != "" && strings.ToLower(strings.TrimSpace(candidate.InstanceType)) == currentInstanceType {
			s += 1
		}
		if currentSource != "" && strings.ToLower(strings.TrimSpace(candidate.Source)) == currentSource {
			s += 1
		}
		if currentDestination != "" && strings.ToLower(strings.TrimSpace(candidate.Destination)) == currentDestination {
			s += 1
		}
		candidateKeywords := observationKeywordSet(candidate.KeyObservations)
		overlap := keywordSetOverlap(currentKeywords, candidateKeywords)
		s += overlap
		if currentService != "" && candidateService != "" {
			if !exactServiceMatch && (strings.Contains(candidateService, currentService) || strings.Contains(currentService, candidateService)) {
				s += 1
			}
		}
		pathMatch := (currentSource != "" && strings.ToLower(strings.TrimSpace(candidate.Source)) == currentSource) ||
			(currentDestination != "" && strings.ToLower(strings.TrimSpace(candidate.Destination)) == currentDestination)
		if s >= 3 && (overlap > 0 || pathMatch) {
			candidates = append(candidates, scored{id: id, score: s})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.id)
	}
	if len(out) > MaxSymptomRelatedCaseIDs {
		out = out[:MaxSymptomRelatedCaseIDs]
	}
	return uniqueNonEmpty(out)
}

func observationKeywordSet(observations []AIOPSKeyObservation) map[string]struct{} {
	out := make(map[string]struct{})
	for _, obs := range observations {
		for _, kw := range obs.Keyword {
			k := strings.ToLower(strings.TrimSpace(kw))
			if k != "" {
				out[k] = struct{}{}
			}
		}
	}
	return out
}

func keywordSetOverlap(a, b map[string]struct{}) int {
	count := 0
	for k := range a {
		if _, ok := b[k]; ok {
			count++
		}
	}
	return count
}

func unionIDs(groups ...[]string) []string {
	merged := make([]string, 0)
	for _, group := range groups {
		merged = append(merged, group...)
	}
	return uniqueNonEmpty(merged)
}

func appendEvalNotes(base string, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	switch {
	case base == "":
		return extra
	case extra == "":
		return base
	default:
		return base + "; " + extra
	}
}

func joinTopKeywords(keywords []string, limit int) string {
	items := uniqueNonEmpty(keywords)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return strings.Join(items, " ")
}

func observationKeywords(observations []AIOPSKeyObservation) []string {
	var out []string
	for _, obs := range observations {
		out = append(out, strings.TrimSpace(obs.Type))
		out = append(out, obs.Keyword...)
	}
	return uniqueNonEmpty(out)
}

func joinKeywords(values []string) string {
	return strings.Join(uniqueNonEmpty(values), ", ")
}

func uniqueNonEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func fallback(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func nonEmptyOr(value string, fallbackValue string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallbackValue
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir json dir: %w", err)
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json %s: %w", path, err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write json %s: %w", path, err)
	}
	return nil
}
