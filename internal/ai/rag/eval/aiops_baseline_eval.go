package eval

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

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
