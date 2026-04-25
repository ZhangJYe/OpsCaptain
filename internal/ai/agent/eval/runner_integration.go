//go:build agent_eval_integration

package eval

import (
	"SuperBizAgent/internal/ai/agent/reporter"
	"SuperBizAgent/internal/ai/agent/skillspecialists/knowledge"
	"SuperBizAgent/internal/ai/agent/skillspecialists/logs"
	"SuperBizAgent/internal/ai/agent/skillspecialists/metrics"
	"SuperBizAgent/internal/ai/agent/supervisor"
	"SuperBizAgent/internal/ai/agent/triage"
	"SuperBizAgent/internal/ai/runtime"
)

func NewMultiAgentRunner() (*MultiAgentRunner, error) {
	rt := runtime.New()
	for _, agent := range []runtime.Agent{
		supervisor.New(),
		triage.New(),
		metrics.New(),
		logs.New(),
		knowledge.New(),
		reporter.New(),
	} {
		if err := rt.Register(agent); err != nil {
			return nil, err
		}
	}
	return NewRuntimeRunner(rt), nil
}
