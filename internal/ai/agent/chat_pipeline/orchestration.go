package chat_pipeline

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func BuildChatAgent(ctx context.Context) (r compose.Runnable[*UserMessage, *schema.Message], err error) {
	return BuildChatAgentWithQuery(ctx, "")
}

func BuildChatAgentWithQuery(ctx context.Context, query string) (r compose.Runnable[*UserMessage, *schema.Message], err error) {
	const (
		ChatTemplate = "ChatTemplate"
		ReactAgent   = "ReactAgent"
		InputToChat  = "InputToChat"
	)
	g := compose.NewGraph[*UserMessage, *schema.Message]()
	chatTemplateKeyOfChatTemplate, err := newChatTemplate(ctx)
	if err != nil {
		return nil, err
	}
	if err = g.AddChatTemplateNode(ChatTemplate, chatTemplateKeyOfChatTemplate); err != nil {
		return nil, fmt.Errorf("add ChatTemplate node: %w", err)
	}
	reactAgentKeyOfLambda, err := newReactAgentLambdaWithQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	if err = g.AddLambdaNode(ReactAgent, reactAgentKeyOfLambda, compose.WithNodeName("ReActAgent")); err != nil {
		return nil, fmt.Errorf("add ReactAgent node: %w", err)
	}
	if err = g.AddLambdaNode(InputToChat, compose.InvokableLambdaWithOption(newInputToChatLambda), compose.WithNodeName("UserMessageToChat")); err != nil {
		return nil, fmt.Errorf("add InputToChat node: %w", err)
	}
	if err = g.AddEdge(compose.START, InputToChat); err != nil {
		return nil, fmt.Errorf("add START->InputToChat edge: %w", err)
	}
	if err = g.AddEdge(ReactAgent, compose.END); err != nil {
		return nil, fmt.Errorf("add ReactAgent->END edge: %w", err)
	}
	if err = g.AddEdge(InputToChat, ChatTemplate); err != nil {
		return nil, fmt.Errorf("add InputToChat->ChatTemplate edge: %w", err)
	}
	if err = g.AddEdge(ChatTemplate, ReactAgent); err != nil {
		return nil, fmt.Errorf("add ChatTemplate->ReactAgent edge: %w", err)
	}
	r, err = g.Compile(ctx, compose.WithGraphName("ChatAgent"), compose.WithNodeTriggerMode(compose.AllPredecessor))
	if err != nil {
		return nil, err
	}
	return r, err
}
