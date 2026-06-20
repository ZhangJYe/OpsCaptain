package actionexecutor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	action := &ActionDefinition{
		ID:       "test_action",
		Name:     "Test Action",
		Category: CategoryQuery,
		RiskLevel: RiskLow,
		Executor: "http",
	}
	r.Register(action)

	got, ok := r.Get("test_action")
	assert.True(t, ok)
	assert.Equal(t, "test_action", got.ID)
}

func TestRegistry_ListByCategory(t *testing.T) {
	r := NewRegistry()
	r.Register(&ActionDefinition{ID: "a1", Category: CategoryQuery})
	r.Register(&ActionDefinition{ID: "a2", Category: CategoryRestart})
	r.Register(&ActionDefinition{ID: "a3", Category: CategoryQuery})

	query := r.ListByCategory(CategoryQuery)
	assert.Len(t, query, 2)
}

func TestRegistry_Execute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	r := NewRegistry()
	r.RegisterExecutor("http", NewHTTPExecutor())
	r.Register(&ActionDefinition{
		ID:       "test_get",
		Name:     "Test GET",
		Category: CategoryQuery,
		RiskLevel: RiskLow,
		Executor: "http",
		Config:   map[string]string{"method": "GET", "url": server.URL + "/test"},
	})

	result, err := r.Execute(context.Background(), "test_get", nil)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "ok")
	assert.Equal(t, "test_get", result.ActionID)
	assert.GreaterOrEqual(t, result.Duration, int64(0))
}

func TestRegistry_Execute_MissingRequired(t *testing.T) {
	r := NewRegistry()
	r.Register(&ActionDefinition{
		ID:       "test_required",
		Executor: "http",
		Parameters: []ActionParam{
			{Name: "service", Required: true, Description: "service name"},
		},
	})

	_, err := r.Execute(context.Background(), "test_required", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required parameter")
}

func TestRegistry_Execute_WithDefaults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("default applied"))
	}))
	defer server.Close()

	r := NewRegistry()
	r.RegisterExecutor("http", NewHTTPExecutor())
	r.Register(&ActionDefinition{
		ID:       "test_default",
		Category: CategoryQuery,
		Executor: "http",
		Config:   map[string]string{"method": "GET", "url": server.URL + "/test"},
		Parameters: []ActionParam{
			{Name: "namespace", Default: "default"},
		},
	})

	result, err := r.Execute(context.Background(), "test_default", nil)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestRegistry_Execute_NotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute(context.Background(), "nonexistent", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestHTTPExecutor_URLTemplate(t *testing.T) {
	var receivedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	exec := NewHTTPExecutor()
	action := &ActionDefinition{
		ID:       "test_template",
		Executor: "http",
		Config:   map[string]string{"method": "GET", "url": server.URL + "/api/{namespace}/{service}"},
	}

	params := map[string]string{"namespace": "prod", "service": "paymentservice"}
	_, err := exec.Execute(context.Background(), action, params)
	require.NoError(t, err)
	assert.Equal(t, "/api/prod/paymentservice", receivedURL)
}

func TestHTTPExecutor_PostWithBody(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("created"))
	}))
	defer server.Close()

	exec := NewHTTPExecutor()
	action := &ActionDefinition{
		ID:       "test_post",
		Executor: "http",
		Config:   map[string]string{"method": "POST", "url": server.URL + "/create"},
	}

	params := map[string]string{"body": `{"name":"test"}`}
	result, err := exec.Execute(context.Background(), action, params)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, `{"name":"test"}`, receivedBody)
}

func TestHTTPExecutor_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	exec := NewHTTPExecutor()
	action := &ActionDefinition{
		ID:       "test_error",
		Executor: "http",
		Config:   map[string]string{"method": "GET", "url": server.URL + "/error"},
	}

	result, err := exec.Execute(context.Background(), action, nil)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, "500", result.Metadata["status_code"])
}
