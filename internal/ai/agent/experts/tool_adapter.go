package experts

import (
	"context"

	einoschema "github.com/cloudwego/eino/schema"

	"github.com/cloudwego/eino/components/tool"
)

type ToolAdapter struct {
	tool       tool.InvokableTool
	toolInfo   *einoschema.ToolInfo
	name       string
	argBuilder ArgBuilder
}

func NewToolAdapter(name string, t tool.InvokableTool) (*ToolAdapter, error) {
	info, err := t.Info(context.Background())
	if err != nil {
		return nil, err
	}
	return &ToolAdapter{
		tool:       t,
		toolInfo:   info,
		name:       name,
		argBuilder: GetArgBuilder(name),
	}, nil
}

func (a *ToolAdapter) Run(ctx context.Context, naturalLanguageArgs string) (string, error) {
	jsonArgs, err := a.argBuilder.Build(naturalLanguageArgs)
	if err != nil {
		return "", err
	}
	return a.tool.InvokableRun(ctx, jsonArgs)
}

func (a *ToolAdapter) Name() string {
	return a.name
}
