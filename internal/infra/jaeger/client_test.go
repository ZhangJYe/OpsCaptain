package jaeger

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDependencies_Success(t *testing.T) {
	expected := []DependencyEdge{
		{Parent: "gateway", Child: "paymentservice", CallCount: 1234},
		{Parent: "paymentservice", Child: "userservice", CallCount: 890},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/dependencies", r.URL.Path)
		assert.Contains(t, r.URL.Query().Get("endTs"), "")
		assert.NotEmpty(t, r.URL.Query().Get("lookback"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	edges, err := client.GetDependencies(context.Background(), 24)

	require.NoError(t, err)
	require.Len(t, edges, 2)
	assert.Equal(t, "gateway", edges[0].Parent)
	assert.Equal(t, "paymentservice", edges[0].Child)
	assert.Equal(t, int64(1234), edges[0].CallCount)
	assert.Equal(t, "paymentservice", edges[1].Parent)
	assert.Equal(t, "userservice", edges[1].Child)
	assert.Equal(t, int64(890), edges[1].CallCount)
}

func TestGetDependencies_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	edges, err := client.GetDependencies(context.Background(), 24)

	require.NoError(t, err)
	assert.Len(t, edges, 0)
}

func TestGetDependencies_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	edges, err := client.GetDependencies(context.Background(), 24)

	assert.Error(t, err)
	assert.Nil(t, edges)
	assert.Contains(t, err.Error(), "500")
}

func TestGetDependencies_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	edges, err := client.GetDependencies(context.Background(), 24)

	assert.Error(t, err)
	assert.Nil(t, edges)
	assert.Contains(t, err.Error(), "decode")
}

func TestGetDependencies_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewClient(server.URL)
	edges, err := client.GetDependencies(ctx, 24)

	assert.Error(t, err)
	assert.Nil(t, edges)
}

func TestGetDependencies_DefaultLookback(t *testing.T) {
	var capturedLookback string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedLookback = r.URL.Query().Get("lookback")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetDependencies(context.Background(), 0)
	require.NoError(t, err)
	assert.Equal(t, "86400000", capturedLookback)
}
