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

//go:embed agent_route.txt
var AgentRoute string

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

//go:embed gos_ingest.txt
var GOSIngest string

//go:embed gos_planner.txt
var GOSPlanner string

//go:embed gos_expert_system.txt
var GOSExpertSystem string

//go:embed gos_expert_tool_call.txt
var GOSExpertToolCall string

//go:embed gos_expert_retrieve.txt
var GOSExpertRetrieve string

//go:embed gos_expert_analyze.txt
var GOSExpertAnalyze string

func init() {
	ChatBase = strings.TrimSpace(ChatBase)
	ChatIdentity = strings.TrimSpace(ChatIdentity)
	ChatLanguage = strings.TrimSpace(ChatLanguage)
	ChatEvidence = strings.TrimSpace(ChatEvidence)
	ChatRuntimeContext = strings.TrimSpace(ChatRuntimeContext)
	AgentRoute = strings.TrimSpace(AgentRoute)
	RAGRewrite = strings.TrimSpace(RAGRewrite)
	RAGRerank = strings.TrimSpace(RAGRerank)
	RAGPlannerSystem = strings.TrimSpace(RAGPlannerSystem)
	RAGEvaluator = strings.TrimSpace(RAGEvaluator)
	RAGPlannerAgent = strings.TrimSpace(RAGPlannerAgent)
	GOSIngest = strings.TrimSpace(GOSIngest)
	GOSPlanner = strings.TrimSpace(GOSPlanner)
	GOSExpertSystem = strings.TrimSpace(GOSExpertSystem)
	GOSExpertToolCall = strings.TrimSpace(GOSExpertToolCall)
	GOSExpertRetrieve = strings.TrimSpace(GOSExpertRetrieve)
	GOSExpertAnalyze = strings.TrimSpace(GOSExpertAnalyze)
}
