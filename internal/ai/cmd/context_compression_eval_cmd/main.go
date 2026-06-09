package main

import (
	"SuperBizAgent/internal/ai/contextcompression"
	"SuperBizAgent/internal/ai/memory"
	"SuperBizAgent/utility/common"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EvalSample 评测样本
type EvalSample struct {
	ID               string   `json:"id"`
	SourceType       string   `json:"source_type"`
	SourceID         string   `json:"source_id"`
	Query            string   `json:"query"`
	Content          string   `json:"content"`
	RequiredEvidence []string `json:"required_evidence"`
}

// EvalResult 单个样本的评测结果
type EvalResult struct {
	ID                      string  `json:"id"`
	SourceType              string  `json:"source_type"`
	SourceID                string  `json:"source_id"`
	Mode                    string  `json:"mode"`
	Strategy                string  `json:"strategy"`
	TokensBefore            int     `json:"tokens_before"`
	CandidateTokensAfter    int     `json:"candidate_tokens_after"`
	RuntimeTokensAfter      int     `json:"runtime_tokens_after"`
	CandidateSavingRatio    float64 `json:"candidate_saving_ratio"`
	RuntimeSavingRatio      float64 `json:"runtime_saving_ratio"`
	CandidateEvidenceRecall float64 `json:"candidate_evidence_recall"`
	RuntimeEvidenceRecall   float64 `json:"runtime_evidence_recall"`
	LatencyMs               int64   `json:"latency_ms"`
	EvidenceBefore          int     `json:"evidence_before"`
	CandidateEvidenceAfter  int     `json:"candidate_evidence_after"`
	RuntimeEvidenceAfter    int     `json:"runtime_evidence_after"`
	Degraded                bool    `json:"degraded"`
	ContentBefore           string  `json:"content_before,omitempty"`
	CandidateContentAfter   string  `json:"candidate_content_after,omitempty"`
	RuntimeContentAfter     string  `json:"runtime_content_after,omitempty"`
}

// EvalSummary 评测汇总
type EvalSummary struct {
	Cases                   int            `json:"cases"`
	AvgTokensBefore         int            `json:"avg_tokens_before"`
	AvgCandidateTokensAfter int            `json:"avg_candidate_tokens_after"`
	AvgRuntimeTokensAfter   int            `json:"avg_runtime_tokens_after"`
	AvgCandidateSavingRatio float64        `json:"avg_candidate_saving_ratio"`
	AvgRuntimeSavingRatio   float64        `json:"avg_runtime_saving_ratio"`
	P95LatencyMs            int64          `json:"p95_latency_ms"`
	AvgLatencyMs            float64        `json:"avg_latency_ms"`
	CandidateEvidenceRecall float64        `json:"candidate_evidence_recall"`
	RuntimeEvidenceRecall   float64        `json:"runtime_evidence_recall"`
	Degraded                int            `json:"degraded"`
	Strategies              map[string]int `json:"strategies"`
}

// EvalReport 完整评测报告
type EvalReport struct {
	Mode      string       `json:"mode"`
	Timestamp string       `json:"timestamp"`
	Summary   EvalSummary  `json:"summary"`
	Results   []EvalResult `json:"results"`
}

func main() {
	inputPath := flag.String("input", "evals/context_compression/samples.jsonl", "样本文件路径")
	modeRaw := flag.String("mode", "audit", "压缩模式: audit,optimize")
	outPath := flag.String("out", "", "输出报告路径")
	showContent := flag.Bool("show-content", false, "报告中包含压缩前后内容")
	gate := flag.Bool("gate", false, "启用门禁：未达 evidence/saving/latency 阈值时返回非零状态")
	minRuntimeSaving := flag.Float64("min-runtime-saving", 0.25, "gate: optimize 模式最小 runtime token 节省率")
	minCandidateEvidence := flag.Float64("min-candidate-evidence", 0.95, "gate: 最小 candidate evidence recall")
	maxP95LatencyMs := flag.Int64("max-p95-latency-ms", 100, "gate: 最大 p95 压缩延迟")
	flag.Parse()

	if err := common.LoadPreferredEnvFile(); err != nil {
		fmt.Printf("WARNING: 加载 env 文件失败: %v\n", err)
	}

	modes := strings.Split(*modeRaw, ",")

	samples, err := loadSamples(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载样本失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("加载 %d 个样本，模式: %s\n", len(samples), *modeRaw)

	allReports := make(map[string]*EvalReport)
	for _, mode := range modes {
		mode = strings.TrimSpace(mode)
		if mode != "audit" && mode != "optimize" {
			fmt.Fprintf(os.Stderr, "未知模式: %s\n", mode)
			continue
		}
		fmt.Printf("\n=== 模式: %s ===\n", mode)
		report := runEval(samples, mode, *showContent)
		allReports[mode] = report
		printSummary(mode, report.Summary)
		if *gate && !gatePassed(mode, report.Summary, *minRuntimeSaving, *minCandidateEvidence, *maxP95LatencyMs) {
			fmt.Fprintf(os.Stderr, "context compression gate failed for mode=%s\n", mode)
			os.Exit(1)
		}
	}

	if strings.TrimSpace(*outPath) != "" {
		if len(allReports) == 1 {
			// 单模式，直接输出报告
			for _, report := range allReports {
				writeReport(report, *outPath)
			}
		} else {
			// 多模式，输出 map
			data, err := json.MarshalIndent(allReports, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "序列化失败: %v\n", err)
				os.Exit(1)
			}
			os.MkdirAll(filepath.Dir(*outPath), 0o755)
			os.WriteFile(*outPath, append(data, '\n'), 0o644)
			fmt.Printf("\n报告已保存到 %s\n", *outPath)
		}
	}
}

func loadSamples(path string) ([]EvalSample, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var samples []EvalSample
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var s EvalSample
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			fmt.Printf("WARNING: 解析样本行失败: %v\n", err)
			continue
		}
		samples = append(samples, s)
	}
	return samples, nil
}

func runEval(samples []EvalSample, mode string, showContent bool) *EvalReport {
	ctx := context.Background()
	cfg := &contextcompression.CompressionConfig{
		Enabled:         true,
		Mode:            contextcompression.Mode(mode),
		MinTokens:       100,
		PreserveFirst:   3,
		PreserveLast:    2,
		LogContextLines: 1,
		SourceTypes:     []string{"tool", "rag"},
	}

	report := &EvalReport{
		Mode:      mode,
		Timestamp: time.Now().Format(time.RFC3339),
		Results:   make([]EvalResult, 0, len(samples)),
		Summary:   EvalSummary{Strategies: make(map[string]int)},
	}

	var latencies []int64
	totalCandidateSavingRatio := 0.0
	totalRuntimeSavingRatio := 0.0
	totalCandidateEvidenceRecall := 0.0
	totalRuntimeEvidenceRecall := 0.0

	for i, sample := range samples {
		fmt.Printf("  [%d/%d] %s ... ", i+1, len(samples), sample.ID)

		result := contextcompression.Compress(ctx, contextcompression.Request{
			SourceType: contextcompression.SourceType(sample.SourceType),
			SourceID:   sample.SourceID,
			Query:      sample.Query,
			Content:    sample.Content,
		}, cfg)

		evidenceBefore := countEvidence(sample.Content, sample.RequiredEvidence)
		candidateEvidenceAfter := countEvidence(result.CandidateContent, sample.RequiredEvidence)
		runtimeEvidenceAfter := countEvidence(result.Content, sample.RequiredEvidence)
		candidateEvidenceRecall := 1.0
		runtimeEvidenceRecall := 1.0
		if evidenceBefore > 0 {
			candidateEvidenceRecall = float64(candidateEvidenceAfter) / float64(evidenceBefore)
			runtimeEvidenceRecall = float64(runtimeEvidenceAfter) / float64(evidenceBefore)
		}
		tokensBefore := result.Report.TokensBefore
		candidateTokensAfter := memory.EstimateTokens(result.CandidateContent)
		runtimeTokensAfter := memory.EstimateTokens(result.Content)
		candidateSavingRatio := savingRatio(tokensBefore, candidateTokensAfter)
		runtimeSavingRatio := savingRatio(tokensBefore, runtimeTokensAfter)

		evalResult := EvalResult{
			ID:                      sample.ID,
			SourceType:              sample.SourceType,
			SourceID:                sample.SourceID,
			Mode:                    mode,
			Strategy:                result.Report.Strategy,
			TokensBefore:            tokensBefore,
			CandidateTokensAfter:    candidateTokensAfter,
			RuntimeTokensAfter:      runtimeTokensAfter,
			CandidateSavingRatio:    candidateSavingRatio,
			RuntimeSavingRatio:      runtimeSavingRatio,
			CandidateEvidenceRecall: candidateEvidenceRecall,
			RuntimeEvidenceRecall:   runtimeEvidenceRecall,
			LatencyMs:               result.Report.LatencyMs,
			EvidenceBefore:          evidenceBefore,
			CandidateEvidenceAfter:  candidateEvidenceAfter,
			RuntimeEvidenceAfter:    runtimeEvidenceAfter,
			Degraded:                result.Report.Degraded,
		}
		if showContent {
			evalResult.ContentBefore = sample.Content
			evalResult.CandidateContentAfter = result.CandidateContent
			evalResult.RuntimeContentAfter = result.Content
		}

		report.Results = append(report.Results, evalResult)
		latencies = append(latencies, result.Report.LatencyMs)
		totalCandidateSavingRatio += candidateSavingRatio
		totalRuntimeSavingRatio += runtimeSavingRatio
		totalCandidateEvidenceRecall += candidateEvidenceRecall
		totalRuntimeEvidenceRecall += runtimeEvidenceRecall

		if result.Report.Strategy != "" {
			report.Summary.Strategies[result.Report.Strategy]++
		}
		if result.Report.Degraded {
			report.Summary.Degraded++
		}

		fmt.Printf("strategy=%s before=%d candidate_after=%d runtime_after=%d candidate_save=%.2f runtime_save=%.2f candidate_evidence=%.2f runtime_evidence=%.2f latency=%dms\n",
			result.Report.Strategy, tokensBefore, candidateTokensAfter, runtimeTokensAfter,
			candidateSavingRatio, runtimeSavingRatio, candidateEvidenceRecall, runtimeEvidenceRecall, result.Report.LatencyMs)
	}

	// 计算汇总指标
	report.Summary.Cases = len(samples)
	if len(samples) > 0 {
		totalBefore := 0
		totalCandidateAfter := 0
		totalRuntimeAfter := 0
		for _, r := range report.Results {
			totalBefore += r.TokensBefore
			totalCandidateAfter += r.CandidateTokensAfter
			totalRuntimeAfter += r.RuntimeTokensAfter
		}
		report.Summary.AvgTokensBefore = totalBefore / len(samples)
		report.Summary.AvgCandidateTokensAfter = totalCandidateAfter / len(samples)
		report.Summary.AvgRuntimeTokensAfter = totalRuntimeAfter / len(samples)
		report.Summary.AvgCandidateSavingRatio = totalCandidateSavingRatio / float64(len(samples))
		report.Summary.AvgRuntimeSavingRatio = totalRuntimeSavingRatio / float64(len(samples))
		report.Summary.CandidateEvidenceRecall = totalCandidateEvidenceRecall / float64(len(samples))
		report.Summary.RuntimeEvidenceRecall = totalRuntimeEvidenceRecall / float64(len(samples))
		report.Summary.AvgLatencyMs = avgLatency(latencies)
		report.Summary.P95LatencyMs = p95Latency(latencies)
	}

	return report
}

func savingRatio(before, after int) float64 {
	if before <= 0 {
		return 0
	}
	saving := float64(before-after) / float64(before)
	if saving < 0 {
		return 0
	}
	return saving
}

func countEvidence(content string, evidence []string) int {
	if len(evidence) == 0 {
		return 0
	}
	contentLower := strings.ToLower(content)
	count := 0
	for _, e := range evidence {
		if strings.Contains(contentLower, strings.ToLower(e)) {
			count++
		}
	}
	return count
}

func avgLatency(latencies []int64) float64 {
	if len(latencies) == 0 {
		return 0
	}
	var sum int64
	for _, l := range latencies {
		sum += l
	}
	return float64(sum) / float64(len(latencies))
}

func p95Latency(latencies []int64) int64 {
	if len(latencies) == 0 {
		return 0
	}
	// 简单排序取 p95
	sorted := make([]int64, len(latencies))
	copy(sorted, latencies)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	idx := int(float64(len(sorted)) * 0.95)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func printSummary(mode string, summary EvalSummary) {
	fmt.Printf("\n========================================\n")
	fmt.Printf("  Context Compression Eval Report (%s)\n", mode)
	fmt.Printf("========================================\n")
	fmt.Printf("  Cases          : %d\n", summary.Cases)
	fmt.Printf("  Avg Before     : %d tokens\n", summary.AvgTokensBefore)
	fmt.Printf("  Candidate After: %d tokens\n", summary.AvgCandidateTokensAfter)
	fmt.Printf("  Runtime After  : %d tokens\n", summary.AvgRuntimeTokensAfter)
	fmt.Printf("  Candidate Save : %.2f%%\n", summary.AvgCandidateSavingRatio*100)
	fmt.Printf("  Runtime Save   : %.2f%%\n", summary.AvgRuntimeSavingRatio*100)
	fmt.Printf("  P95 Latency    : %d ms\n", summary.P95LatencyMs)
	fmt.Printf("  Avg Latency    : %.1f ms\n", summary.AvgLatencyMs)
	fmt.Printf("  Candidate Evidence Recall: %.2f%%\n", summary.CandidateEvidenceRecall*100)
	fmt.Printf("  Runtime Evidence Recall  : %.2f%%\n", summary.RuntimeEvidenceRecall*100)
	fmt.Printf("  Degraded       : %d\n", summary.Degraded)
	fmt.Printf("  Strategies     : %v\n", summary.Strategies)
	fmt.Printf("========================================\n")
}

func gatePassed(mode string, summary EvalSummary, minRuntimeSaving, minCandidateEvidence float64, maxP95LatencyMs int64) bool {
	if summary.CandidateEvidenceRecall < minCandidateEvidence {
		return false
	}
	if summary.P95LatencyMs > maxP95LatencyMs {
		return false
	}
	if summary.Degraded > 0 {
		return false
	}
	if mode == "optimize" && summary.AvgRuntimeSavingRatio < minRuntimeSaving {
		return false
	}
	return true
}

func writeReport(report *EvalReport, outPath string) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "序列化报告失败: %v\n", err)
		os.Exit(1)
	}
	os.MkdirAll(filepath.Dir(outPath), 0o755)
	data = append(data, '\n')
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "写入报告失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n报告已保存到 %s\n", outPath)
}

// EstimateTokens 用于 eval 的 token 估算（复用 memory 包的逻辑）
func EstimateTokens(text string) int {
	return memory.EstimateTokens(text)
}
