package main

import (
	tools2 "SuperBizAgent/internal/ai/tools"
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

func main() {
	ctx := context.Background()
	modelVal, err := g.Cfg().Get(ctx, "glm_chat_model_fast.model")
	if err != nil {
		panic(err)
	}
	apiKeyVal, err := g.Cfg().Get(ctx, "glm_chat_model_fast.api_key")
	if err != nil {
		panic(err)
	}
	baseURLVal, err := g.Cfg().Get(ctx, "glm_chat_model_fast.base_url")
	if err != nil {
		panic(err)
	}
	config := &openai.ChatModelConfig{
		APIKey:  apiKeyVal.String(),
		Model:   modelVal.String(),
		BaseURL: baseURLVal.String(),
	}
	chatModel, err := openai.NewChatModel(ctx, config)
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

	err = chatModel.BindTools(toolInfos)
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
