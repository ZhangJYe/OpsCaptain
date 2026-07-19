package experts

import (
	"context"
	"time"

	"SuperBizAgent/internal/ai/belief"

	einomodel "github.com/cloudwego/eino/components/model"
	einoschema "github.com/cloudwego/eino/schema"
)

type RAGQueryFunc func(ctx context.Context, query string) ([]*einoschema.Document, error)

type GenerateContentFunc func(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph, history []RetrievalRecord, decision map[string]string) (string, error)
type ChatModelFactory func(ctx context.Context) (einomodel.ToolCallingChatModel, error)

type ExpertRuntimeConfig struct {
	Name                string
	Description         string
	ToolNames           []string
	MaxRetrievalSteps   int
	ModelPath           string
	Temperature         float64
	MaxTokens           int
	EvidenceMaxChars    int
	EvidenceMaxItems    int
	RAGQueryFunc        RAGQueryFunc
	GenerateContentFunc GenerateContentFunc
	ChatModelFactory    ChatModelFactory
	CallTimeout         time.Duration
	ExecutionBudget     ExecutionBudget
}

type RetrievalRecord struct {
	Query  string
	Output string
	Tool   string
}

type ToolOutput struct {
	Success           bool        `json:"success"`
	Degraded          bool        `json:"degraded"`
	Error             string      `json:"error"`
	IsError           bool        `json:"isError"`
	Content           interface{} `json:"content"`
	Data              interface{} `json:"data"`
	HasExplicitFields bool        `json:"-"`
	HasSuccess        bool        `json:"-"`
}

type LinuxSREExpert struct {
	*BaseExpert
}

func NewLinuxSREExpert(cfg ExpertRuntimeConfig, toolReg *ToolRegistry) *LinuxSREExpert {
	return &LinuxSREExpert{
		BaseExpert: NewBaseExpert(cfg, toolReg),
	}
}

type NetworkSREExpert struct {
	*BaseExpert
}

func NewNetworkSREExpert(cfg ExpertRuntimeConfig, toolReg *ToolRegistry) *NetworkSREExpert {
	return &NetworkSREExpert{
		BaseExpert: NewBaseExpert(cfg, toolReg),
	}
}

type DatabaseSREExpert struct {
	*BaseExpert
}

func NewDatabaseSREExpert(cfg ExpertRuntimeConfig, toolReg *ToolRegistry) *DatabaseSREExpert {
	return &DatabaseSREExpert{
		BaseExpert: NewBaseExpert(cfg, toolReg),
	}
}
