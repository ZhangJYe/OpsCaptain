package promptreg

import (
	_ "embed"
	"strings"
)

//go:embed chat_base.txt
var ChatBase string

//go:embed chat_identity.txt
var ChatIdentity string

//go:embed chat_language.txt
var ChatLanguage string

//go:embed chat_evidence.txt
var ChatEvidence string

//go:embed chat_runtime_context.txt
var ChatRuntimeContext string

//go:embed rag_rewrite.txt
var RAGRewrite string

//go:embed rag_rerank.txt
var RAGRerank string

//go:embed rag_planner.txt
var RAGPlannerSystem string

//go:embed rag_evaluator.txt
var RAGEvaluator string

//go:embed rag_planner_agent.txt
var RAGPlannerAgent string

func init() {
	ChatBase = strings.TrimSpace(ChatBase)
	ChatIdentity = strings.TrimSpace(ChatIdentity)
	ChatLanguage = strings.TrimSpace(ChatLanguage)
	ChatEvidence = strings.TrimSpace(ChatEvidence)
	ChatRuntimeContext = strings.TrimSpace(ChatRuntimeContext)
	RAGRewrite = strings.TrimSpace(RAGRewrite)
	RAGRerank = strings.TrimSpace(RAGRerank)
	RAGPlannerSystem = strings.TrimSpace(RAGPlannerSystem)
	RAGEvaluator = strings.TrimSpace(RAGEvaluator)
	RAGPlannerAgent = strings.TrimSpace(RAGPlannerAgent)
}
