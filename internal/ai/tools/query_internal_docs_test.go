package tools

import (
	"SuperBizAgent/internal/ai/rag"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

type fakeInternalDocsRetriever struct{}

func (f *fakeInternalDocsRetriever) Retrieve(context.Context, string, ...retrieverapi.Option) ([]*schema.Document, error) {
	return []*schema.Document{}, nil
}

type fakeInternalDocsErrorRetriever struct {
	err error
}

func (f *fakeInternalDocsErrorRetriever) Retrieve(context.Context, string, ...retrieverapi.Option) ([]*schema.Document, error) {
	return nil, f.err
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
		output, err := tool.InvokableRun(context.Background(), input)
		if err != nil {
			t.Fatalf("expected degraded output on run %d, got error: %v", i+1, err)
		}
		var payload QueryInternalDocsOutput
		if err := json.Unmarshal([]byte(output), &payload); err != nil {
			t.Fatalf("run %d: failed to parse degraded output %q: %v", i+1, output, err)
		}
		if payload.Success {
			t.Fatalf("run %d: expected success=false, got %#v", i+1, payload)
		}
		if payload.Error == "" {
			t.Fatalf("run %d: expected error detail, got %#v", i+1, payload)
		}
	}
	if created != 1 {
		t.Fatalf("expected failed retriever init to be cached, got %d creations", created)
	}
}

func TestQueryInternalDocsToolReturnsSchemaMismatchDegradedPayload(t *testing.T) {
	oldFactory := rag.NewRetrieverFunc
	defer func() {
		rag.NewRetrieverFunc = oldFactory
		rag.ResetSharedPool()
	}()

	rag.ResetSharedPool()
	rag.NewRetrieverFunc = func(context.Context) (retrieverapi.Retriever, error) {
		return &fakeInternalDocsErrorRetriever{err: errors.New("milvus collection schema mismatch: missing field content")}, nil
	}

	tool := NewQueryInternalDocsTool()
	output, err := tool.InvokableRun(context.Background(), `{"query":"部署回滚"}`)
	if err != nil {
		t.Fatalf("expected degraded output, got error: %v", err)
	}
	var payload QueryInternalDocsOutput
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("failed to parse payload %q: %v", output, err)
	}
	if payload.Success || !payload.Degraded {
		t.Fatalf("expected degraded payload, got %#v", payload)
	}
	if !strings.Contains(payload.Error, "schema mismatch") {
		t.Fatalf("expected schema mismatch error, got %q", payload.Error)
	}
}
