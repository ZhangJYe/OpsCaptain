package gos_engine

import "SuperBizAgent/internal/ai/belief"

type Config struct {
	Enabled           bool           `yaml:"enabled"`
	ModelPath         string         `yaml:"model_path"`
	Temperature       float64        `yaml:"temperature"`
	MaxTokens         int            `yaml:"max_tokens"`
	SessionMaxSteps   int            `yaml:"session_max_steps"`
	MaxRetrievalSteps int            `yaml:"max_retrieval_steps"`
	FSM               FSMConfig      `yaml:"fsm"`
	Experts           []ExpertConfig `yaml:"experts"`
	HeadAgent         string         `yaml:"head_agent"`
}

type FSMConfig struct {
	GapDelta   float64 `yaml:"gap_delta"`
	MinSupport int     `yaml:"min_support"`
	MaxSteps   int     `yaml:"max_steps"`
}

type ExpertConfig struct {
	Name              string   `yaml:"name"`
	Description       string   `yaml:"description"`
	Tools             []string `yaml:"tools"`
	MaxRetrievalSteps int      `yaml:"max_retrieval_steps"`
}

func DefaultConfig() *Config {
	return &Config{
		Enabled:           false,
		ModelPath:         "deepseek-v3",
		Temperature:       0.8,
		MaxTokens:         4096,
		SessionMaxSteps:   5,
		MaxRetrievalSteps: 3,
		FSM: FSMConfig{
			GapDelta:   0.3,
			MinSupport: 2,
			MaxSteps:   3,
		},
		Experts: []ExpertConfig{
			{
				Name:              "linux_sre",
				Description:       "Linux SRE 专家",
				Tools:             []string{"query_logs", "query_internal_docs"},
				MaxRetrievalSteps: 3,
			},
			{
				Name:              "network_sre",
				Description:       "网络 SRE 专家",
				Tools:             []string{"query_logs", "query_internal_docs"},
				MaxRetrievalSteps: 3,
			},
			{
				Name:              "database_sre",
				Description:       "数据库 SRE 专家",
				Tools:             []string{"query_logs", "query_internal_docs"},
				MaxRetrievalSteps: 3,
			},
		},
		HeadAgent: "sre_commander",
	}
}

func (c *Config) ToFSMThresholds() belief.FSMThresholds {
	return belief.FSMThresholds{
		GapDelta:   c.FSM.GapDelta,
		MinSupport: c.FSM.MinSupport,
		MaxSteps:   c.FSM.MaxSteps,
	}
}
