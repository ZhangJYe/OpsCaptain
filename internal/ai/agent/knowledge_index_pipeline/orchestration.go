package knowledge_index_pipeline

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
)

func BuildKnowledgeIndexing(ctx context.Context) (r compose.Runnable[document.Source, []string], err error) {
	const (
		FileLoader       = "FileLoader"
		MarkdownSplitter = "MarkdownSplitter"
		MilvusIndexer    = "MilvusIndexer"
	)
	g := compose.NewGraph[document.Source, []string]()
	fileLoaderKeyOfLoader, err := newLoader(ctx)
	if err != nil {
		return nil, err
	}
	if err = g.AddLoaderNode(FileLoader, fileLoaderKeyOfLoader); err != nil {
		return nil, fmt.Errorf("add FileLoader node: %w", err)
	}
	markdownSplitterKeyOfDocumentTransformer, err := newDocumentTransformer(ctx)
	if err != nil {
		return nil, err
	}
	if err = g.AddDocumentTransformerNode(MarkdownSplitter, markdownSplitterKeyOfDocumentTransformer); err != nil {
		return nil, fmt.Errorf("add MarkdownSplitter node: %w", err)
	}
	milvusIndexerKeyOfIndexer, err := newIndexer(ctx)
	if err != nil {
		return nil, err
	}
	if err = g.AddIndexerNode(MilvusIndexer, milvusIndexerKeyOfIndexer); err != nil {
		return nil, fmt.Errorf("add MilvusIndexer node: %w", err)
	}
	if err = g.AddEdge(compose.START, FileLoader); err != nil {
		return nil, fmt.Errorf("add START->FileLoader edge: %w", err)
	}
	if err = g.AddEdge(MilvusIndexer, compose.END); err != nil {
		return nil, fmt.Errorf("add MilvusIndexer->END edge: %w", err)
	}
	if err = g.AddEdge(FileLoader, MarkdownSplitter); err != nil {
		return nil, fmt.Errorf("add FileLoader->MarkdownSplitter edge: %w", err)
	}
	if err = g.AddEdge(MarkdownSplitter, MilvusIndexer); err != nil {
		return nil, fmt.Errorf("add MarkdownSplitter->MilvusIndexer edge: %w", err)
	}
	r, err = g.Compile(ctx, compose.WithGraphName("KnowledgeIndexing"), compose.WithNodeTriggerMode(compose.AnyPredecessor))
	if err != nil {
		return nil, err
	}
	return r, err
}
