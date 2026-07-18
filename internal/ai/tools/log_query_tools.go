package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	e_mcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type LogQueryInput struct {
	Query   string `json:"query" jsonschema:"description=日志检索关键词或查询语句，例如 checkout timeout"`
	Service string `json:"service,omitempty" jsonschema:"description=可选的服务名，例如 checkout、payment、gateway"`
	Window  string `json:"window,omitempty" jsonschema:"description=可选的时间范围，例如 最近30分钟、1h"`
}

type LogQueryUnavailableOutput struct {
	Success  bool   `json:"success"`
	Degraded bool   `json:"degraded"`
	Message  string `json:"message"`
	Error    string `json:"error,omitempty"`
	Query    string `json:"query,omitempty"`
	Service  string `json:"service,omitempty"`
	Window   string `json:"window,omitempty"`
}

func logToolOrDegraded(t tool.InvokableTool, reason string) tool.InvokableTool {
	if t != nil {
		return t
	}
	return &degradedLogTool{reason: reason}
}

type degradedLogTool struct {
	reason string
}

func (d *degradedLogTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "query_logs", Desc: "log tool degraded: " + d.reason}, nil
}

func (d *degradedLogTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	out := LogQueryUnavailableOutput{
		Success: false, Degraded: true,
		Message: "日志检索工具创建失败，已返回降级结果。",
		Error:   d.reason,
	}
	data, _ := json.Marshal(out)
	return string(data), nil
}

func NewUnavailableLogQueryTool(reason string) tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_logs",
		"Query application logs. When the real log MCP service is unavailable, this tool returns a structured degraded result instead of leaving the agent without a log tool.",
		func(ctx context.Context, input *LogQueryInput, opts ...tool.Option) (string, error) {
			if input == nil {
				input = &LogQueryInput{}
			}
			out := LogQueryUnavailableOutput{
				Success:  false,
				Degraded: true,
				Message:  "日志检索工具当前不可用，已返回降级结果。请基于可用告警、知识库和用户提供的上下文继续分析，并明确标注缺少实时日志证据。",
				Error:    reason,
				Query:    input.Query,
				Service:  input.Service,
				Window:   input.Window,
			}
			data, marshalErr := json.Marshal(out)
			if marshalErr != nil {
				return "", marshalErr
			}
			return string(data), nil
		},
	)
	if err != nil {
		g.Log().Errorf(context.Background(), "failed to create query_logs fallback tool: %v", err)
		return nil
	}
	return t
}

func NewHTTPLogQueryTool(httpURL string, reason string) tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_logs",
		"Query application logs through the HTTP fallback endpoint when the SSE MCP transport is unavailable.",
		func(ctx context.Context, input *LogQueryInput, opts ...tool.Option) (string, error) {
			if input == nil {
				input = &LogQueryInput{}
			}
			applyDefaultLogWindow(input, g.Cfg().MustGet(ctx, "mcp.log_default_window", "30m").String())
			payload, err := json.Marshal(input)
			if err != nil {
				return "", err
			}
			result, err := callLogHTTPFallback(ctx, httpURL, string(payload), time.Duration(defaultToolTimeoutMs)*time.Millisecond)
			if err != nil {
				unavail := NewUnavailableLogQueryTool(fmt.Sprintf("%s; http fallback failed: %v", reason, err))
				if unavail == nil {
					return "", fmt.Errorf("log http fallback failed and degraded tool unavailable: %w", err)
				}
				return unavail.InvokableRun(ctx, string(payload))
			}
			return result, nil
		},
	)
	if err != nil {
		g.Log().Errorf(context.Background(), "failed to create query_logs http fallback tool: %v", err)
		return nil
	}
	return t
}

func applyDefaultLogWindow(input *LogQueryInput, window string) {
	if input == nil || strings.TrimSpace(input.Window) != "" {
		return
	}
	input.Window = strings.TrimSpace(window)
}

func callLogHTTPFallback(ctx context.Context, httpURL string, args string, timeout time.Duration) (string, error) {
	if strings.TrimSpace(httpURL) == "" {
		return "", fmt.Errorf("log http fallback url is empty")
	}
	if timeout <= 0 {
		timeout = defaultToolTimeoutMs * time.Millisecond
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body := strings.TrimSpace(args)
	if body == "" {
		body = "{}"
	}
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, httpURL, bytes.NewBufferString(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("log http fallback status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if len(data) == 0 {
		return "{}", nil
	}
	return string(data), nil
}

func resolveLogHTTPURL(ctx context.Context, mcpURL string) string {
	if value := normalizeOptionalURL(os.Getenv("MCP_LOG_HTTP_URL")); value != "" {
		return value
	}
	if v, err := g.Cfg().Get(ctx, "mcp.log_http_url"); err == nil {
		if value := normalizeOptionalURL(v.String()); value != "" {
			return value
		}
	}
	mcpURL = normalizeOptionalURL(mcpURL)
	if mcpURL == "" {
		return ""
	}
	parsed, err := url.Parse(mcpURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.Path = "/tools/query_logs"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// --- 工具发现结果缓存 ---

const toolCacheErrorTTL = 5 * time.Minute // 错误缓存 TTL，过期后自动重试

type cachedTools struct {
	tools    []tool.BaseTool
	err      error
	cachedAt time.Time
	isError  bool
}

var (
	toolCache   map[string]*cachedTools
	toolCacheMu sync.RWMutex
)

func getCachedTools(url string) (*cachedTools, bool) {
	toolCacheMu.RLock()
	defer toolCacheMu.RUnlock()
	if toolCache == nil {
		return nil, false
	}
	c, ok := toolCache[url]
	if !ok {
		return nil, false
	}
	// 错误缓存过期后自动失效，允许重试
	if c.isError && time.Since(c.cachedAt) > toolCacheErrorTTL {
		return nil, false
	}
	return c, true
}

func setCachedTools(url string, ct *cachedTools) {
	ct.cachedAt = time.Now()
	ct.isError = ct.err != nil
	toolCacheMu.Lock()
	defer toolCacheMu.Unlock()
	if toolCache == nil {
		toolCache = make(map[string]*cachedTools)
	}
	toolCache[url] = ct
}

// GetLogMcpTool 获取日志 MCP 工具，带连接池复用、超时保护、断线重连、结果缓存
func GetLogMcpTool() ([]tool.BaseTool, error) {
	ctx := context.Background()
	mcpURL := ""
	if v, err := g.Cfg().Get(ctx, "mcp.log_url"); err == nil {
		mcpURL = normalizeOptionalURL(v.String())
	}
	// fallback: 直接读环境变量（Docker env_file 注入的变量可能不会被 GoFrame ${} 替换）
	if mcpURL == "" {
		mcpURL = normalizeOptionalURL(os.Getenv("MCP_LOG_URL"))
	}
	if mcpURL == "" {
		httpURL := resolveLogHTTPURL(ctx, "")
		if httpURL != "" {
			g.Log().Warning(ctx, "mcp.log_url is not configured, log query tool will use HTTP fallback")
			return []tool.BaseTool{logToolOrDegraded(NewHTTPLogQueryTool(httpURL, "mcp.log_url is not configured"), "mcp.log_url is not configured")}, nil
		}
		g.Log().Warning(ctx, "mcp.log_url is not configured, log query tool will use degraded fallback")
		return []tool.BaseTool{logToolOrDegraded(NewUnavailableLogQueryTool("mcp.log_url is not configured"), "mcp.log_url is not configured")}, nil
	}
	httpURL := resolveLogHTTPURL(ctx, mcpURL)
	if httpURL != "" && g.Cfg().MustGet(ctx, "mcp.prefer_http_log_tool", false).Bool() {
		return []tool.BaseTool{logToolOrDegraded(NewHTTPLogQueryTool(httpURL, "mcp.prefer_http_log_tool is enabled"), "mcp.prefer_http_log_tool is enabled")}, nil
	}

	// 检查缓存
	if ct, ok := getCachedTools(mcpURL); ok {
		return ct.tools, ct.err
	}

	connectTimeoutMs := g.Cfg().MustGet(ctx, "mcp.connect_timeout_ms", defaultConnectTimeoutMs).Int()
	toolTimeoutMs := g.Cfg().MustGet(ctx, "mcp.tool_timeout_ms", defaultToolTimeoutMs).Int()

	// 获取复用的连接池客户端
	pc, err := globalPool.getOrCreate(mcpURL, connectTimeoutMs, toolTimeoutMs)
	if err != nil {
		if httpURL != "" {
			return []tool.BaseTool{logToolOrDegraded(NewHTTPLogQueryTool(httpURL, err.Error()), err.Error())}, nil
		}
		return []tool.BaseTool{logToolOrDegraded(NewUnavailableLogQueryTool(err.Error()), err.Error())}, nil
	}

	// 用 eino 适配器发现工具（获取完整 schema），带超时保护避免 SSE 卡死阻塞启动
	listCtx, listCancel := context.WithTimeout(ctx, time.Duration(toolTimeoutMs)*time.Millisecond)
	defer listCancel()
	einoTools, err := e_mcp.GetTools(listCtx, &e_mcp.Config{Cli: pc.cli})
	if err != nil {
		if httpURL != "" {
			return []tool.BaseTool{logToolOrDegraded(NewHTTPLogQueryTool(httpURL, fmt.Sprintf("failed to get MCP tools: %v", err)), fmt.Sprintf("failed to get MCP tools: %v", err))}, nil
		}
		return []tool.BaseTool{logToolOrDegraded(NewUnavailableLogQueryTool(fmt.Sprintf("failed to get MCP tools: %v", err)), fmt.Sprintf("failed to get MCP tools: %v", err))}, nil
	}

	// 包装每个工具，实际调用走连接池（超时 + 重连），缓存工具名
	var tools []tool.BaseTool
	hasQueryLogsAlias := false
	for _, t := range einoTools {
		if it, ok := t.(tool.InvokableTool); ok {
			info, _ := it.Info(ctx)
			name := ""
			if info != nil {
				name = info.Name
			}
			if name == "query_logs" {
				hasQueryLogsAlias = true
			}
			tools = append(tools, &pooledToolWrapper{inner: it, pool: pc, toolName: name, httpURL: httpURL})
		} else {
			tools = append(tools, t)
		}
	}
	if !hasQueryLogsAlias {
		for idx, t := range tools {
			it, ok := t.(*pooledToolWrapper)
			if !ok {
				continue
			}
			it.alias = "query_logs"
			tools[idx] = it
			hasQueryLogsAlias = true
			g.Log().Infof(ctx, "MCP log tool alias applied: actual=%s alias=query_logs", it.toolName)
			break
		}
	}

	setCachedTools(mcpURL, &cachedTools{tools: tools})
	g.Log().Infof(ctx, "MCP log tools ready: url=%s tools=%d connect_timeout=%dms tool_timeout=%dms",
		mcpURL, len(tools), connectTimeoutMs, toolTimeoutMs)
	return tools, nil
}

func normalizeOptionalURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "${") && strings.Contains(value, "}") {
		return ""
	}
	return value
}
