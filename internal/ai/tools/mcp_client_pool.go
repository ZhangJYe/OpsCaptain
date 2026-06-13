package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultConnectTimeoutMs = 10000
	defaultToolTimeoutMs    = 5000
	maxReconnectRetries     = 3
	reconnectBaseDelay      = time.Second
)

// mcpClientPool 按 URL 复用 MCP 客户端，避免重复建连
type mcpClientPool struct {
	mu      sync.RWMutex
	clients map[string]*pooledClient
}

type pooledClient struct {
	cli              client.MCPClient
	url              string
	connected        bool
	rw               sync.RWMutex // 保护 cli/connected 的读写
	mu               sync.Mutex   // 保护 reconnect 操作（防并发重连）
	toolTimeoutMs    int
	connectTimeoutMs int
}

var globalPool = &mcpClientPool{
	clients: make(map[string]*pooledClient),
}

func (p *mcpClientPool) getOrCreate(url string, connectTimeoutMs, toolTimeoutMs int) (*pooledClient, error) {
	p.mu.RLock()
	pc, ok := p.clients[url]
	p.mu.RUnlock()
	if ok {
		return pc, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// double check
	if pc, ok = p.clients[url]; ok {
		return pc, nil
	}

	cli, err := p.connect(url, connectTimeoutMs)
	if err != nil {
		return nil, err
	}

	pc = &pooledClient{
		cli:              cli,
		url:              url,
		connected:        true,
		toolTimeoutMs:    toolTimeoutMs,
		connectTimeoutMs: connectTimeoutMs,
	}
	p.clients[url] = pc
	return pc, nil
}

func (p *mcpClientPool) connect(url string, connectTimeoutMs int) (client.MCPClient, error) {
	cli, err := client.NewSSEMCPClient(url)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP SSE client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(connectTimeoutMs)*time.Millisecond)
	defer cancel()

	if err = cli.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start MCP client (timeout %dms): %w", connectTimeoutMs, err)
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "superbizagent-client",
		Version: "1.0.0",
	}
	if _, err = cli.Initialize(ctx, initRequest); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return cli, nil
}

// reconnect 断线重连，指数退避
// mu 防止并发重连，rw 保护 cli/connected 更新
func (pc *pooledClient) reconnect(ctx context.Context) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// 检查是否已被其他 goroutine 重连成功
	pc.rw.RLock()
	if pc.connected {
		pc.rw.RUnlock()
		return nil
	}
	pc.rw.RUnlock()

	var lastErr error
	for i := 0; i < maxReconnectRetries; i++ {
		delay := reconnectBaseDelay * time.Duration(1<<uint(i))
		if delay > 60*time.Second {
			delay = 60 * time.Second
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}

		cli, err := globalPool.connect(pc.url, pc.connectTimeoutMs)
		if err != nil {
			lastErr = err
			continue
		}

		// 重连成功，写锁更新状态
		pc.rw.Lock()
		pc.cli = cli
		pc.connected = true
		pc.rw.Unlock()
		return nil
	}

	return fmt.Errorf("MCP reconnect failed after %d retries: %w", maxReconnectRetries, lastErr)
}

// CallTool 带超时和自动重连的工具调用
func (pc *pooledClient) CallTool(ctx context.Context, toolName string, argsJSON string) (string, error) {
	timeout := time.Duration(pc.toolTimeoutMs) * time.Millisecond

	result, err := pc.doCall(ctx, toolName, argsJSON, timeout)
	if err != nil && isConnectionError(err) {
		pc.rw.Lock()
		pc.connected = false
		pc.rw.Unlock()

		g.Log().Warningf(ctx, "MCP tool %s connection lost, attempting reconnect...", toolName)
		if reconnectErr := pc.reconnect(ctx); reconnectErr != nil {
			return "", fmt.Errorf("MCP tool %s call failed and reconnect failed: %w (original: %v)", toolName, reconnectErr, err)
		}
		// 重连成功，重试一次
		result, err = pc.doCall(ctx, toolName, argsJSON, timeout)
	}

	if err != nil {
		return "", fmt.Errorf("MCP tool %s call failed: %w", toolName, err)
	}

	if result.IsError {
		return "", fmt.Errorf("MCP tool %s returned error: %v", toolName, result.Content)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("MCP tool %s result marshal failed: %w", toolName, err)
	}
	return string(raw), nil
}

func (pc *pooledClient) doCall(ctx context.Context, toolName string, argsJSON string, timeout time.Duration) (*mcp.CallToolResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pc.rw.RLock()
	cli := pc.cli
	pc.rw.RUnlock()

	return cli.CallTool(callCtx, mcp.CallToolRequest{
		Request: mcp.Request{Method: "tools/call"},
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: []byte(argsJSON),
		},
	})
}

// isConnectionError 判断是否为连接层错误（不是业务超时）
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// 排除 context.DeadlineExceeded（工具执行超时，重连无意义）
	if msg == context.DeadlineExceeded.Error() {
		return false
	}
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "reset by peer") ||
		strings.Contains(msg, "unknown session") ||
		strings.Contains(msg, "transport error") ||
		strings.Contains(msg, "request failed with status 404") ||
		strings.Contains(msg, "status 404")
}

// pooledToolWrapper 包装 eino MCP 工具，调用走连接池（带超时 + 重连）
type pooledToolWrapper struct {
	inner    tool.InvokableTool
	pool     *pooledClient
	toolName string // 缓存工具名，避免每次调用都查 Info
	alias    string
	httpURL  string
}

func (w *pooledToolWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	info, err := w.inner.Info(ctx)
	if err != nil {
		return nil, err
	}
	if w.alias == "" {
		return info, nil
	}
	if info == nil {
		return &schema.ToolInfo{Name: w.alias}, nil
	}
	cloned := *info
	cloned.Name = w.alias
	return &cloned, nil
}

func (w *pooledToolWrapper) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	result, err := w.pool.CallTool(ctx, w.toolName, args)
	if err == nil {
		return result, nil
	}
	if w.httpURL == "" || !isConnectionError(err) {
		return "", err
	}
	fallbackResult, fallbackErr := callLogHTTPFallback(ctx, w.httpURL, args, time.Duration(w.pool.toolTimeoutMs)*time.Millisecond)
	if fallbackErr != nil {
		return "", fmt.Errorf("%w; http fallback failed: %v", err, fallbackErr)
	}
	return fallbackResult, nil
}