package main

import (
	"SuperBizAgent/internal/ai/models"
	tools2 "SuperBizAgent/internal/ai/tools"
	"SuperBizAgent/utility/common"
	"context"
	"fmt"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func main() {
	if err := common.LoadPreferredEnvFile(); err != nil {
		panic(err)
	}
	ctx := context.Background()
	chatModel, err := models.OpenAIForGLMFast(ctx)
	if err != nil {
		panic(err)
	}
	toolList, _ := tools2.GetLogMcpTool()
	toolList = append(toolList, tools2.NewGetCurrentTimeTool())
	toolInfos := make([]*schema.ToolInfo, 0)
	var info *schema.ToolInfo
	for _, todoTool := range toolList {
		info, err = todoTool.Info(ctx)
		if err != nil {
			panic(err)
		}
		toolInfos = append(toolInfos, info)
	}

	chatModel, err = chatModel.WithTools(toolInfos)
	if err != nil {
		panic(err)
	}

	chain := compose.NewChain[[]*schema.Message, *schema.Message]()
	chain.AppendChatModel(chatModel, compose.WithNodeName("chat_model"))

	agent, err := chain.Compile(ctx)
	if err != nil {
		panic(err)
	}
	resp, err := agent.Invoke(ctx, []*schema.Message{
		{
			Role:    schema.User,
			Content: "告诉我你有哪些工具可以使用",
		},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.Content)
}
