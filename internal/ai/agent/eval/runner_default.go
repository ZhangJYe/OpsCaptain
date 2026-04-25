//go:build !agent_eval_integration

package eval

import "fmt"

func NewMultiAgentRunner() (*MultiAgentRunner, error) {
	return nil, fmt.Errorf("build with -tags agent_eval_integration to create a registered multi-agent runner")
}
