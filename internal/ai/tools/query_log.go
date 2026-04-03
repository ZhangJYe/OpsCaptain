package tools

import (
	"context"
	"fmt"

	e_mcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

/*
	GetLogMcpTool

https://cloud.tencent.com/developer/mcp/server/11710
https://cloud.tencent.com/document/product/614/118699#90415b66-8edb-43a9-ad5a-c2b0a97f5eaf

https://www.cloudwego.io/zh/docs/eino/ecosystem_integration/tool/tool_mcp/
https://mcp-go.dev/clients
*/
func GetLogMcpTool() ([]tool.BaseTool, error) {
	ctx := context.Background()
	mcpURLVal, err := g.Cfg().Get(ctx, "mcp.log_url")
	if err != nil {
		return nil, fmt.Errorf("failed to read mcp.log_url from config: %w", err)
	}
	mcpURL := mcpURLVal.String()
	if mcpURL == "" {
		return nil, fmt.Errorf("mcp.log_url is not configured, please set it in config.yaml")
	}
	cli, err := client.NewSSEMCPClient(mcpURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP SSE client: %w", err)
	}
	err = cli.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start MCP client: %w", err)
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "superbizagent-client",
		Version: "1.0.0",
	}
	if _, err = cli.Initialize(ctx, initRequest); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}
	mcpTools, err := e_mcp.GetTools(ctx, &e_mcp.Config{Cli: cli})
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP tools: %w", err)
	}
	return mcpTools, nil
}
