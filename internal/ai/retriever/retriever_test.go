package retriever

import (
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/common"
	"context"
	"errors"
	"strings"
	"testing"

	milvusretriever "github.com/cloudwego/eino-ext/components/retriever/milvus"
	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

type fakeSafeRetrieverInner struct {
	docs    []*schema.Document
	err     error
	errs    []error
	calls   int
	filters []string
}

func (f *fakeSafeRetrieverInner) Retrieve(_ context.Context, _ string, opts ...retrieverapi.Option) ([]*schema.Document, error) {
	f.calls++
	io := retrieverapi.GetImplSpecificOptions(&milvusretriever.ImplOptions{}, opts...)
	f.filters = append(f.filters, io.Filter)
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.docs, nil
}

func TestFloatVectorConverter_ProducesMilvusFloatVectors(t *testing.T) {
	vectors, err := floatVectorConverter(context.Background(), [][]float64{{1.5, -2.25, 3}})
	if err != nil {
		t.Fatalf("floatVectorConverter returned error: %v", err)
	}
	if len(vectors) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vectors))
	}

	floatVector, ok := vectors[0].(entity.FloatVector)
	if !ok {
		t.Fatalf("expected entity.FloatVector, got %T", vectors[0])
	}
	if len(floatVector) != 3 {
		t.Fatalf("expected len 3, got %d", len(floatVector))
	}
	if floatVector[0] != float32(1.5) || floatVector[1] != float32(-2.25) || floatVector[2] != float32(3) {
		t.Fatalf("unexpected float vector: %#v", floatVector)
	}
}

func TestResolveMilvusSearchParam_HNSW(t *testing.T) {
	param, err := resolveMilvusSearchParam(context.Background(), 5)
	if err != nil {
		t.Fatalf("resolveMilvusSearchParam returned error: %v", err)
	}
	if _, ok := param.(*entity.IndexHNSWSearchParam); !ok {
		t.Fatalf("expected *entity.IndexHNSWSearchParam, got %T", param)
	}
}

func TestSafeRetrieverReturnsSchemaMismatch(t *testing.T) {
	r := &safeRetriever{
		inner: &fakeSafeRetrieverInner{
			err: errors.New("extra output fields [content metadata] found and result does not dynamic field"),
		},
	}

	_, err := r.Retrieve(context.Background(), "部署回滚")
	if err == nil {
		t.Fatal("expected schema mismatch error")
	}
	if !strings.Contains(err.Error(), "milvus collection schema mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSafeRetrieverAddsOwnerScopeFilter(t *testing.T) {
	inner := &fakeSafeRetrieverInner{docs: []*schema.Document{{ID: "doc-1"}}}
	r := &safeRetriever{inner: inner}
	ctx := context.WithValue(context.Background(), consts.CtxKeyUserID, "alice@example.com")

	_, err := r.Retrieve(ctx, "redis", retrieverapi.WithTopK(3))
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(inner.filters) != 1 {
		t.Fatalf("expected 1 call, got %d", len(inner.filters))
	}
	alicePrefix := common.KnowledgeSourcePrefixForUser("alice@example.com")
	if !strings.Contains(inner.filters[0], `metadata["_source"] like "`+alicePrefix+`%`) {
		t.Fatalf("expected filter to include alice prefix, got %s", inner.filters[0])
	}
	if !strings.Contains(inner.filters[0], `not metadata["_source"] like "upload://users/%"`) {
		t.Fatalf("expected filter to allow legacy sources, got %s", inner.filters[0])
	}
	if strings.Contains(inner.filters[0], "alice@example.com") {
		t.Fatalf("filter should not expose raw user id: %s", inner.filters[0])
	}
}

func TestSafeRetrieverSkipsOwnerScopeFilterForAnonymous(t *testing.T) {
	inner := &fakeSafeRetrieverInner{docs: []*schema.Document{{ID: "doc-1"}}}
	r := &safeRetriever{inner: inner}

	_, err := r.Retrieve(context.Background(), "redis")
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(inner.filters) != 1 || inner.filters[0] != "" {
		t.Fatalf("expected no filter for anonymous context, got %#v", inner.filters)
	}
}

func TestSafeRetrieverRetriesWithoutOwnerScopeFilterOnExpressionError(t *testing.T) {
	inner := &fakeSafeRetrieverInner{
		docs: []*schema.Document{{ID: "doc-1"}},
		errs: []error{errors.New("failed to create query plan: cannot parse expression")},
	}
	r := &safeRetriever{inner: inner}
	ctx := context.WithValue(context.Background(), consts.CtxKeyUserID, "alice@example.com")

	docs, err := r.Retrieve(ctx, "redis")
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected retry docs, got %d", len(docs))
	}
	if inner.calls != 2 {
		t.Fatalf("expected retry without filter, got %d calls", inner.calls)
	}
	if inner.filters[0] == "" || inner.filters[1] != "" {
		t.Fatalf("expected first call scoped and second unscoped, got %#v", inner.filters)
	}
}
