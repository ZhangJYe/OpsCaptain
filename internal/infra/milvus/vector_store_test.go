package milvus

import (
	"context"
	"fmt"
	"testing"

	cli "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

func TestEscapeMilvusStringLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{`has "quotes"`, `has \"quotes\"`},
		{`has \backslash`, `has \\backslash`},
		{`has \"both`, `has \\\"both`},
		{"", ""},
		{`C:\Users\test`, `C:\\Users\\test`},
		{`path/"with"/\special`, `path/\"with\"/\\special`},
	}
	for _, tt := range tests {
		got := escapeMilvusStringLiteral(tt.input)
		if got != tt.want {
			t.Errorf("escapeMilvusStringLiteral(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

type mockQueryDeleter struct {
	queryResult cli.ResultSet
	queryErr    error
	deleteErr   error
	queriedExpr string
	deletedExpr string
}

func (m *mockQueryDeleter) Query(_ context.Context, _ string, _ []string, expr string, _ []string, _ ...cli.SearchQueryOptionFunc) (cli.ResultSet, error) {
	m.queriedExpr = expr
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return m.queryResult, nil
}

func (m *mockQueryDeleter) Delete(_ context.Context, _, _, expr string) error {
	m.deletedExpr = expr
	return m.deleteErr
}

func TestDeleteBySource_EscapesSourceValue(t *testing.T) {
	mock := &mockQueryDeleter{
		queryResult: cli.ResultSet{
			entity.NewColumnVarChar("id", []string{"doc-1"}),
		},
	}
	store := &MilvusVectorStore{
		newClient: func(_ context.Context) (milvusQueryDeleter, error) { return mock, nil },
	}

	n, err := store.DeleteBySource(context.Background(), "test", `path/with"quotes`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 deleted, got %d", n)
	}
	if mock.deletedExpr != `id in ["doc-1"]` {
		t.Errorf("deleted expr = %q, want %q", mock.deletedExpr, `id in ["doc-1"]`)
	}
	wantQuery := `metadata["_source"] == "path/with\"quotes"`
	if mock.queriedExpr != wantQuery {
		t.Errorf("query expr = %q, want %q", mock.queriedExpr, wantQuery)
	}
}

func TestDeleteBySource_EscapesIDsWithQuotes(t *testing.T) {
	mock := &mockQueryDeleter{
		queryResult: cli.ResultSet{
			entity.NewColumnVarChar("id", []string{`id"with"quotes`}),
		},
	}
	store := &MilvusVectorStore{
		newClient: func(_ context.Context) (milvusQueryDeleter, error) { return mock, nil },
	}

	_, err := store.DeleteBySource(context.Background(), "test", "normal-source")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantExpr := `id in ["id\"with\"quotes"]`
	if mock.deletedExpr != wantExpr {
		t.Errorf("deleted expr = %q, want %q", mock.deletedExpr, wantExpr)
	}
}

func TestDeleteBySource_PropagatesDeleteError(t *testing.T) {
	mock := &mockQueryDeleter{
		queryResult: cli.ResultSet{
			entity.NewColumnVarChar("id", []string{"doc-1"}),
		},
		deleteErr: fmt.Errorf("milvus unavailable"),
	}
	store := &MilvusVectorStore{
		newClient: func(_ context.Context) (milvusQueryDeleter, error) { return mock, nil },
	}

	_, err := store.DeleteBySource(context.Background(), "test", "source")
	if err == nil {
		t.Fatal("expected error from delete, got nil")
	}
}

func TestDeleteBySource_ReturnsZeroWhenNoMatches(t *testing.T) {
	mock := &mockQueryDeleter{
		queryResult: cli.ResultSet{},
	}
	store := &MilvusVectorStore{
		newClient: func(_ context.Context) (milvusQueryDeleter, error) { return mock, nil },
	}

	n, err := store.DeleteBySource(context.Background(), "test", "source")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 deleted, got %d", n)
	}
}

func TestDeleteBySource_PropagatesQueryError(t *testing.T) {
	mock := &mockQueryDeleter{
		queryErr: fmt.Errorf("connection refused"),
	}
	store := &MilvusVectorStore{
		newClient: func(_ context.Context) (milvusQueryDeleter, error) { return mock, nil },
	}

	_, err := store.DeleteBySource(context.Background(), "test", "source")
	if err == nil {
		t.Fatal("expected error from query, got nil")
	}
}

func TestDeleteBySource_PropagatesClientError(t *testing.T) {
	store := &MilvusVectorStore{
		newClient: func(_ context.Context) (milvusQueryDeleter, error) {
			return nil, fmt.Errorf("connect failed")
		},
	}

	_, err := store.DeleteBySource(context.Background(), "test", "source")
	if err == nil {
		t.Fatal("expected error from client factory, got nil")
	}
}
