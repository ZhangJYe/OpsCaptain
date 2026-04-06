package tools

import (
	"SuperBizAgent/internal/ai/rag"
	"context"
	"errors"
	"testing"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

type fakeInternalDocsRetriever struct{}

func (f *fakeInternalDocsRetriever) Retrieve(context.Context, string, ...retrieverapi.Option) ([]*schema.Document, error) {
	return []*schema.Document{}, nil
}

func TestQueryInternalDocsToolReusesRetriever(t *testing.T) {
	oldFactory := rag.NewRetrieverFunc
	defer func() {
		rag.NewRetrieverFunc = oldFactory
		rag.ResetSharedPool()
	}()

	rag.ResetSharedPool()
	created := 0
	rag.NewRetrieverFunc = func(context.Context) (retrieverapi.Retriever, error) {
		created++
		return &fakeInternalDocsRetriever{}, nil
	}

	tool := NewQueryInternalDocsTool()
	input := `{"query":"请查询 SOP"}`
	for i := 0; i < 2; i++ {
		output, err := tool.InvokableRun(context.Background(), input)
		if err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
		if output != "[]" {
			t.Fatalf("expected empty docs output, got %q", output)
		}
	}
	if created != 1 {
		t.Fatalf("expected retriever to be created once, got %d", created)
	}
}

func TestQueryInternalDocsToolCachesRecentInitFailures(t *testing.T) {
	oldFactory := rag.NewRetrieverFunc
	defer func() {
		rag.NewRetrieverFunc = oldFactory
		rag.ResetSharedPool()
	}()

	rag.ResetSharedPool()
	created := 0
	rag.NewRetrieverFunc = func(context.Context) (retrieverapi.Retriever, error) {
		created++
		return nil, errors.New("dial timeout")
	}

	tool := NewQueryInternalDocsTool()
	input := `{"query":"请查询 SOP"}`
	for i := 0; i < 2; i++ {
		if _, err := tool.InvokableRun(context.Background(), input); err == nil {
			t.Fatalf("expected init error on run %d", i+1)
		}
	}
	if created != 1 {
		t.Fatalf("expected failed retriever init to be cached, got %d creations", created)
	}
}
