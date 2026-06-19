package main

import (
	"context"
	"fmt"
	"time"

	"SuperBizAgent/internal/ai/rag"
)

func main() {
	ctx := context.Background()

	// Exactly what agent eval does
	agentCfg := rag.LoadAgentConfig(ctx)
	agentCfg.Enabled = true
	agent := rag.NewAgentRAG(agentCfg)

	fmt.Printf("Agent config: enabled=%v max_rounds=%d eval_timeout=%dms\n",
		agentCfg.Enabled, agentCfg.MaxRounds, agentCfg.EvalTimeoutMs)

	// Test single query
	pool := rag.SharedPool()
	start := time.Now()
	docs, trace, err := agent.Query(ctx, pool, "Prometheus 告警先看什么")
	elapsed := time.Since(start)

	fmt.Printf("Query: docs=%d rounds=%d confidence=%.2f latency=%v err=%v\n",
		len(docs), trace.Rounds, trace.FinalConfidence, elapsed, err)

	if len(docs) > 0 {
		for i, doc := range docs {
			if i >= 3 {
				break
			}
			fmt.Printf("  [%d] %s\n", i, doc.ID)
		}
	}
}
