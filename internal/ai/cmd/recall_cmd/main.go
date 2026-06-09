package main

import (
	retriever2 "SuperBizAgent/internal/ai/retriever"
	inframv "SuperBizAgent/internal/infra/milvus"
	"context"
	"fmt"
)

func main() {
	ctx := context.Background()
	cli, err := inframv.NewMilvusClient(ctx)
	if err != nil {
		panic(err)
	}
	factory := retriever2.NewMilvusRetrieverWithClient(cli)
	r, err := factory(ctx)
	if err != nil {
		panic(err)
	}
	query := "服务下线是什么原因"
	docs, err := r.Retrieve(ctx, query)
	if err != nil {
		panic(err)
	}
	fmt.Println("Q：", query)
	for _, doc := range docs {
		fmt.Println("A：", doc.Content)
	}
	fmt.Println("Done", len(docs))
}
