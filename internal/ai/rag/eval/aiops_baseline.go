package eval

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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
