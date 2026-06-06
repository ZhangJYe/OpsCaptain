# Agent Eval Phase 1: Go 确定性 Gate + Export Runs

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** CI 跑确定性回归 gate（不依赖 LLM API），同时支持导出 GoS/Diag run artifact 供后续 Judge/deepeval 使用。

**Architecture:** 新增 `--mode=gate`（读 baseline 文件，跑 smoke 确定性对比）和 `--mode=export-runs`（运行 GoS 导出结果）。复用现有 `eval.CheckGate`、`eval.NewRunner`、`smokeBaselineRunner` 等组件。

**Tech Stack:** Go, GoFrame, Eino, 现有 `cmd/gos_eval` CLI

---

## 文件结构

| 文件 | 操作 | 说明 |
|------|------|------|
| `evals/baselines/gos_baseline.json` | 创建 | GoS 基准值（入库） |
| `cmd/gos_eval/main.go` | 修改 | 新增 `--mode=gate` 和 `--mode=export-runs` |
| `.gitignore` | 修改 | 排除 evals/runs/、evals/reports/ |
| `Makefile` | 修改 | 新增 eval-gate / eval-runs-gos |
| `.github/workflows/ci.yml` | 修改 | 新增 eval-gate 步骤 |

---

### Task 1: 创建 baseline artifact 和目录结构

**Files:**
- Create: `evals/baselines/gos_baseline.json`
- Create: `evals/runs/.gitkeep`
- Create: `evals/reports/.gitkeep`

- [ ] **Step 1: 创建目录结构**

```bash
mkdir -p evals/baselines evals/runs evals/reports
```

- [ ] **Step 2: 创建 baseline artifact**

从现有 `eval_result.json` 提取 metrics，写入 `evals/baselines/gos_baseline.json`：

```bash
# 读取现有 eval 结果，提取 metrics 部分
cat eval_result.json | python3 -c "
import json, sys
data = json.load(sys.stdin)
baseline = {
    'commit': data.get('commit', ''),
    'model': data.get('model', 'unknown'),
    'tool_config': 'gos_smoke',
    'holdout_path': 'internal/ai/agent/gos_engine/eval/testdata/holdout.json',
    'timestamp': data.get('timestamp', ''),
    'metrics': data.get('gos_metrics', data.get('metrics', {}))
}
json.dump(baseline, sys.stdout, indent=2)
" > evals/baselines/gos_baseline.json
```

如果 `eval_result.json` 不存在或格式不匹配，手动创建：

```json
{
  "commit": "initial",
  "model": "smoke_deterministic",
  "tool_config": "gos_smoke",
  "holdout_path": "internal/ai/agent/gos_engine/eval/testdata/holdout.json",
  "timestamp": "2026-06-06T00:00:00Z",
  "metrics": {
    "total_cases": 5,
    "succeeded": 5,
    "failed": 0,
    "degraded": 0,
    "matched": 5,
    "accuracy": 1.0,
    "evidence_coverage": 1.0,
    "avg_latency": 0,
    "avg_llm_calls": 0,
    "degradation_rate": 0,
    "traceability": 1.0,
    "per_status": {"success": 5}
  }
}
```

- [ ] **Step 3: 创建 .gitkeep 文件**

```bash
touch evals/runs/.gitkeep evals/reports/.gitkeep
```

- [ ] **Step 4: 验证目录结构**

```bash
ls -la evals/baselines/ evals/runs/ evals/reports/
```

Expected: 三个目录都存在，baselines 有 gos_baseline.json，runs 和 reports 有 .gitkeep。

- [ ] **Step 5: Commit**

```bash
git add evals/
git commit -m "feat(eval): 创建 evals 目录结构和 baseline artifact"
```

---

### Task 2: 新增 `--mode=gate`

**Files:**
- Modify: `cmd/gos_eval/main.go:403-428`

`--mode=gate` 的行为：
1. 读取 `--baseline` 指定的 baseline artifact
2. 用 `--gos-profile=eval`（确定性 fake tools）运行 GoS
3. 用 `smokeBaselineRunner`（确定性 keyword 匹配）运行 baseline
4. 调用 `eval.CheckGate(gosMetrics, baselineMetrics)` 对比
5. 输出 gate 报告，失败则 exit 1

与 `--mode=smoke` 的区别：smoke 用内存中的 fake baseline，gate 读文件中的 baseline artifact。

与 `--mode=compare` 的区别：compare 强制 `--gos-profile=real`，gate 强制 `--gos-profile=eval`。

- [ ] **Step 1: 新增 `runGate` 函数**

在 `cmd/gos_eval/main.go` 的 `runSmoke` 函数之后添加：

```go
func runGate(holdoutPath, baselineFile, outputFile string) {
	fmt.Println("=== Eval Gate (确定性回归检查) ===")

	// 1. 读取 baseline artifact
	data, err := os.ReadFile(baselineFile)
	if err != nil {
		fmt.Printf("ERROR: 读取 baseline 失败: %v\n", err)
		os.Exit(1)
	}
	var artifact BaselineArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		fmt.Printf("ERROR: 解析 baseline 失败: %v\n", err)
		os.Exit(1)
	}
	if artifact.Metrics == nil {
		fmt.Println("ERROR: baseline 缺少 metrics")
		os.Exit(1)
	}

	// 2. 用 eval profile（确定性 fake tools）运行 GoS
	engine, _ := buildGoSEngine(true)
	runner := eval.NewRunner(engine)
	start := time.Now()
	gosMetrics, gosResults, err := runner.RunFromFile(context.Background(), holdoutPath)
	if err != nil {
		fmt.Printf("ERROR: GoS 运行失败: %v\n", err)
		os.Exit(1)
	}
	elapsed := time.Since(start)

	// 3. 对比 gate
	gateReport := eval.CheckGate(gosMetrics, artifact.Metrics)

	// 4. 输出结果
	printMetrics("GoS", gosMetrics)
	fmt.Printf("\n--- Gate 结果 ---\n")
	for _, g := range gateReport.Gates {
		status := "PASS"
		if !g.Passed {
			status = "FAIL"
		}
		fmt.Printf("  [%s] %s: expected=%s, actual=%s\n", status, g.Name, g.Expected, g.Actual)
	}
	fmt.Printf("\n总耗时: %v\n", elapsed)

	// 5. 写入输出文件
	output := map[string]interface{}{
		"mode":          "gate",
		"commit":        gitCommit(),
		"baseline_file": baselineFile,
		"gos_metrics":   gosMetrics,
		"gos_results":   gosResults,
		"baseline":      artifact.Metrics,
		"gate":          gateReport,
		"elapsed_ms":    elapsed.Milliseconds(),
	}
	outData, _ := json.MarshalIndent(output, "", "  ")
	if err := os.WriteFile(outputFile, outData, 0o644); err != nil {
		fmt.Printf("WARNING: 写入输出文件失败: %v\n", err)
	}

	if !gateReport.AllPassed {
		fmt.Println("\n❌ Gate 未通过")
		os.Exit(1)
	}
	fmt.Println("\n✅ Gate 通过")
}
```

- [ ] **Step 2: 在 switch 中注册 `gate` 模式**

修改 `cmd/gos_eval/main.go` 的 mode switch（约 line 415）：

```go
switch *mode {
case "gos":
	runGoSOnly(*holdoutPath, *outputFile, *gosProfile)
case "baseline":
	runBaseline(*holdoutPath, *outputFile)
case "compare":
	runCompare(*holdoutPath, *baselineFile, *outputFile, *gosProfile)
case "smoke":
	runSmoke(*holdoutPath, *outputFile)
case "gate":
	runGate(*holdoutPath, *baselineFile, *outputFile)
default:
	fmt.Printf("未知模式: %s\n", *mode)
	fmt.Println("可用模式: gos, baseline, compare, smoke, gate")
	os.Exit(1)
}
```

- [ ] **Step 3: 更新 usage 文字**

修改 flag 定义（约 line 403）：

```go
mode := flag.String("mode", "gos", "运行模式: gos|baseline|compare|smoke|gate")
```

- [ ] **Step 4: 编译验证**

```bash
go build ./cmd/gos_eval/...
```

Expected: 编译成功。

- [ ] **Step 5: Commit**

```bash
git add cmd/gos_eval/main.go
git commit -m "feat(eval): 新增 --mode=gate 确定性回归检查"
```

---

### Task 3: 新增 `--mode=export-runs`

**Files:**
- Modify: `cmd/gos_eval/main.go`

`--mode=export-runs` 的行为：
1. 用 `--gos-profile` 指定的 profile 运行 GoS
2. 输出 `evals/runs/gos_{timestamp}.json`（GoS 结果）
3. 同时输出 `evals/runs/diag_{timestamp}.jsonl`（每行一个 diag case，供 Judge/deepeval 使用）

- [ ] **Step 1: 新增 `runExportRuns` 函数**

在 `runGate` 函数之后添加：

```go
func runExportRuns(holdoutPath, outputDir, gosProfile string) {
	fmt.Println("=== Export Runs ===")

	evalProfile := gosProfile == "eval"
	engine, _ := buildGoSEngine(evalProfile)
	runner := eval.NewRunner(engine)

	metrics, results, err := runner.RunFromFile(context.Background(), holdoutPath)
	if err != nil {
		fmt.Printf("ERROR: GoS 运行失败: %v\n", err)
		os.Exit(1)
	}

	printMetrics("GoS", metrics)

	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Printf("ERROR: 创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	ts := time.Now().Format("20060102150405")

	// 写入 GoS 结果
	gosOutput := map[string]interface{}{
		"mode":        "export-runs",
		"commit":      gitCommit(),
		"profile":     gosProfile,
		"holdout":     holdoutPath,
		"timestamp":   time.Now().Format(time.RFC3339),
		"metrics":     metrics,
		"results":     results,
	}
	gosData, _ := json.MarshalIndent(gosOutput, "", "  ")
	gosFile := filepath.Join(outputDir, fmt.Sprintf("gos_%s.json", ts))
	if err := os.WriteFile(gosFile, gosData, 0o644); err != nil {
		fmt.Printf("WARNING: 写入 GoS 结果失败: %v\n", err)
	} else {
		fmt.Printf("GoS 结果: %s\n", gosFile)
	}

	// 写入 diag runs（JSONL，每行一个 case）
	diagFile := filepath.Join(outputDir, fmt.Sprintf("diag_%s.jsonl", ts))
	var diagLines []string
	for _, r := range results {
		line := map[string]interface{}{
			"case_id":        r.CaseID,
			"query":          r.Symptom,
			"actual_output":  r.Prediction,
			"tools_called":   []string{},     // GoS 不直接暴露工具调用
			"evidence_context": []string{},   // 后续 Judge 可补充
			"latency_ms":     r.Latency.Milliseconds(),
			"llm_calls":      r.LLMCalls,
			"matched":        r.Matched,
			"status":         r.Status,
		}
		lineData, _ := json.Marshal(line)
		diagLines = append(diagLines, string(lineData))
	}
	if err := os.WriteFile(diagFile, []byte(strings.Join(diagLines, "\n")+"\n"), 0o644); err != nil {
		fmt.Printf("WARNING: 写入 diag runs 失败: %v\n", err)
	} else {
		fmt.Printf("Diag runs: %s\n", diagFile)
	}
}
```

- [ ] **Step 2: 在 switch 中注册 `export-runs` 模式**

```go
case "export-runs":
	runExportRuns(*holdoutPath, *outputDir, *gosProfile)
```

- [ ] **Step 3: 新增 `--output-dir` flag**

在 flag 定义区域添加：

```go
outputDir := flag.String("output-dir", "evals/runs", "export-runs 输出目录")
```

- [ ] **Step 4: 更新 usage 文字**

```go
mode := flag.String("mode", "gos", "运行模式: gos|baseline|compare|smoke|gate|export-runs")
```

- [ ] **Step 5: 编译验证**

```bash
go build ./cmd/gos_eval/...
```

Expected: 编译成功。

- [ ] **Step 6: Commit**

```bash
git add cmd/gos_eval/main.go
git commit -m "feat(eval): 新增 --mode=export-runs 导出 GoS/Diag run artifact"
```

---

### Task 4: 更新 .gitignore 和 Makefile

**Files:**
- Modify: `.gitignore`
- Modify: `Makefile`

- [ ] **Step 1: 更新 .gitignore**

在 `.gitignore` 末尾添加：

```
# Eval 运行产物（不入库）
evals/runs/
evals/reports/
evals/deepeval/datasets/
```

但保留 `evals/runs/.gitkeep`（需要 `git add -f` 或在 .gitignore 之前已提交）。

- [ ] **Step 2: 更新 Makefile**

在 Makefile 中添加 eval 相关 target：

```makefile
# === Eval ===

# CI 自动（确定性 smoke gate，无 LLM 依赖）
eval-gate:
	go run cmd/gos_eval/main.go --mode=gate --gos-profile=eval \
	  --baseline=evals/baselines/gos_baseline.json \
	  --output=evals/reports/gate_$$(date +%Y%m%d%H%M%S).json

# 生成 GoS/Diag run artifact（需要 LLM API，手动）
eval-runs-gos:
	go run cmd/gos_eval/main.go --mode=export-runs --gos-profile=eval \
	  --output-dir=evals/runs
```

- [ ] **Step 3: 验证 Makefile**

```bash
make eval-gate
```

Expected: 运行成功（或因 baseline 数据不匹配失败，但不应有编译/运行错误）。

- [ ] **Step 4: Commit**

```bash
git add .gitignore Makefile
git commit -m "feat(eval): 更新 gitignore 和 Makefile eval targets"
```

---

### Task 5: 更新 CI 配置

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: 在 CI 中添加 eval-gate 步骤**

在 `ci.yml` 的 test 步骤之后添加：

```yaml
- name: Eval Gate
  run: make eval-gate
```

- [ ] **Step 2: 验证 CI 配置语法**

```bash
cat .github/workflows/ci.yml | python3 -c "import yaml, sys; yaml.safe_load(sys.stdin)"
```

Expected: 无输出（YAML 语法正确）。

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "feat(eval): CI 新增 eval-gate 步骤"
```

---

### Task 6: 端到端验证

- [ ] **Step 1: 运行 eval-gate**

```bash
make eval-gate
```

Expected: 输出 Gate 结果（PASS 或 FAIL），exit code 与 gate 结果一致。

- [ ] **Step 2: 运行 export-runs**

```bash
make eval-runs-gos
ls -la evals/runs/
```

Expected: 生成 `gos_*.json` 和 `diag_*.jsonl` 文件。

- [ ] **Step 3: 验证 diag JSONL 格式**

```bash
head -1 evals/runs/diag_*.jsonl | python3 -m json.tool
```

Expected: 有效 JSON，包含 `case_id`, `query`, `actual_output`, `evidence_context` 字段。

- [ ] **Step 4: 验证 .gitignore 生效**

```bash
git status evals/runs/ evals/reports/
```

Expected: 只显示 `evals/runs/.gitkeep`（其他文件被忽略）。

- [ ] **Step 5: 最终 Commit**

```bash
git add -A
git commit -m "feat(eval): Phase 1 完成 - 确定性 gate + export-runs"
```
