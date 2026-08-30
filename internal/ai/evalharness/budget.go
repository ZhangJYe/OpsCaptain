package evalharness

import (
	"fmt"
	"sync"
)

type Budget struct {
	mu     sync.Mutex
	limits BudgetLimits
	usage  Usage
	cases  int
}

func NewBudget(limits BudgetLimits) *Budget {
	limits.Normalize()
	return &Budget{limits: limits}
}

func (b *Budget) ReserveCase() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limits.MaxCases > 0 && b.cases >= b.limits.MaxCases {
		return fmt.Errorf("case budget exceeded")
	}
	b.cases++
	return nil
}

func (b *Budget) Add(usage Usage) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.usage.LLMCalls += usage.LLMCalls
	b.usage.ToolCalls += usage.ToolCalls
	b.usage.RAGCalls += usage.RAGCalls
	b.usage.Tokens += usage.Tokens
	b.usage.Cost += usage.Cost
	return b.checkLocked()
}

func (b *Budget) Usage() Usage {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.usage
}

func (b *Budget) checkLocked() error {
	checks := []struct {
		limit int
		used  int
		name  string
	}{
		{b.limits.MaxLLMCalls, b.usage.LLMCalls, "llm call"},
		{b.limits.MaxToolCalls, b.usage.ToolCalls, "tool call"},
		{b.limits.MaxRAGCalls, b.usage.RAGCalls, "rag call"},
		{b.limits.MaxTokens, b.usage.Tokens, "token"},
	}
	for _, check := range checks {
		if check.limit > 0 && check.used > check.limit {
			return fmt.Errorf("%s budget exceeded: %d > %d", check.name, check.used, check.limit)
		}
	}
	if b.limits.MaxCost > 0 && b.usage.Cost > b.limits.MaxCost {
		return fmt.Errorf("cost budget exceeded: %.4f > %.4f", b.usage.Cost, b.limits.MaxCost)
	}
	return nil
}
