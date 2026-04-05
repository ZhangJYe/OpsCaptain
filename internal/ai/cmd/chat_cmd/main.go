package main

import (
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/utility/mem"
	"context"
	"fmt"
)

func main() {
	ctx := context.Background()
	id := "111"
	userMessage := &chat_pipeline.UserMessage{
		ID:      id,
		Query:   "你好",
		History: mem.GetSimpleMemory(id).GetContextMessages(),
	}
	runner, err := chat_pipeline.BuildChatAgent(ctx)
	if err != nil {
		panic(err)
	}
	out, err := runner.Invoke(ctx, userMessage)
	if err != nil {
		panic(err)
	}
	answer := out.Content
	fmt.Println("Q: 你好")
	fmt.Println("A:", answer)
	mem.GetSimpleMemory(id).AddUserAssistantPair("你好", out.Content)
	userMessage = &chat_pipeline.UserMessage{
		ID:      id,
		Query:   "现在是几点",
		History: mem.GetSimpleMemory(id).GetContextMessages(),
	}
	out, err = runner.Invoke(ctx, userMessage)
	if err != nil {
		panic(err)
	}
	answer = out.Content
	fmt.Println("----------------")
	fmt.Println("Q: 现在是几点")
	fmt.Println("A:", answer)
}
