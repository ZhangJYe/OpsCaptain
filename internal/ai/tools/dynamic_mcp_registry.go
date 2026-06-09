package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/skills"

	e_mcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	toolapi "github.com/cloudwego/eino/components/tool"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const defaultDynamicTimeoutMs = 5000

// mcpConn holds a discovered invokable tool and the config that created it.
type mcpConn struct {
	tool      toolapi.InvokableTool
	config    skills.UserMCPTool
	createdAt time.Time
}

// DynamicMCPRegistry manages connections to user-defined MCP servers.
// It discovers tools via the eino MCP adapter and provides thread-safe
// access with timeout and degraded JSON fallback on errors.
type DynamicMCPRegistry struct {
	mu          sync.RWMutex
	connections map[string]*mcpConn
	whitelist   []*net.IPNet
	timeoutMs   int
}

// NewDynamicMCPRegistry creates a registry with a CIDR whitelist.
// An empty whitelist means all endpoints are rejected.
func NewDynamicMCPRegistry(whitelist []string, timeoutMs int) (*DynamicMCPRegistry, error) {
	var nets []*net.IPNet
	for _, cidr := range whitelist {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
		nets = append(nets, ipNet)
	}
	if timeoutMs <= 0 {
		timeoutMs = defaultDynamicTimeoutMs
	}
	return &DynamicMCPRegistry{
		connections: make(map[string]*mcpConn),
		whitelist:   nets,
		timeoutMs:   timeoutMs,
	}, nil
}

// Register connects to the MCP server described by cfg, discovers tools,
// and stores the matching tool in the registry.
func (r *DynamicMCPRegistry) Register(ctx context.Context, cfg skills.UserMCPTool) error {
	// Whitelist check on primary endpoint.
	if err := r.checkWhitelist(cfg.EndpointURL); err != nil {
		return fmt.Errorf("endpoint whitelist check failed: %w", err)
	}
	// Also check HTTPURL if provided.
	if cfg.HTTPURL != "" {
		if err := r.checkWhitelist(cfg.HTTPURL); err != nil {
			return fmt.Errorf("http_url whitelist check failed: %w", err)
		}
	}

	timeoutMs := cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = r.timeoutMs
	}

	invokable, err := r.connectAndDiscover(ctx, cfg, timeoutMs)
	if err != nil {
		return fmt.Errorf("connect and discover failed: %w", err)
	}

	r.mu.Lock()
	r.connections[cfg.ID] = &mcpConn{
		tool:      invokable,
		config:    cfg,
		createdAt: time.Now(),
	}
	r.mu.Unlock()
	return nil
}

// Unregister removes a tool by its ID.
func (r *DynamicMCPRegistry) Unregister(toolID string) {
	r.mu.Lock()
	delete(r.connections, toolID)
	r.mu.Unlock()
}

// Get returns the invokable tool for the given ID.
func (r *DynamicMCPRegistry) Get(toolID string) (toolapi.InvokableTool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conn, ok := r.connections[toolID]
	if !ok {
		return nil, false
	}
	return conn.tool, true
}

// Invoke runs the tool with the given args JSON. On any error it returns
// a degraded JSON response instead of propagating the error.
func (r *DynamicMCPRegistry) Invoke(ctx context.Context, toolID, args string) (string, error) {
	t, ok := r.Get(toolID)
	if !ok {
		return degradedJSON(fmt.Sprintf("tool %q not registered", toolID)), nil
	}

	timeoutMs := r.timeoutMs
	r.mu.RLock()
	if conn, exists := r.connections[toolID]; exists && conn.config.TimeoutMs > 0 {
		timeoutMs = conn.config.TimeoutMs
	}
	r.mu.RUnlock()

	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	result, err := t.InvokableRun(callCtx, args)
	if err != nil {
		return degradedJSON(fmt.Sprintf("invocation failed: %v", err)), nil
	}
	return result, nil
}

// ListConfigs returns the config of every registered tool.
func (r *DynamicMCPRegistry) ListConfigs() []skills.UserMCPTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	configs := make([]skills.UserMCPTool, 0, len(r.connections))
	for _, conn := range r.connections {
		configs = append(configs, conn.config)
	}
	return configs
}

// checkWhitelist verifies that the host in endpointURL resolves to an IP
// allowed by the whitelist. If the whitelist is empty, all endpoints are rejected
// to prevent unauthenticated SSRF.
func (r *DynamicMCPRegistry) checkWhitelist(endpointURL string) error {
	if len(r.whitelist) == 0 {
		return fmt.Errorf("no network whitelist configured; rejecting endpoint %q", endpointURL)
	}
	parsed, err := url.Parse(endpointURL)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", endpointURL, err)
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("no hostname in URL %q", endpointURL)
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		// If DNS lookup fails, try parsing as IP directly.
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("cannot resolve host %q: %w", host, err)
		}
		ips = []net.IP{ip}
	}

	for _, ip := range ips {
		for _, ipNet := range r.whitelist {
			if ipNet.Contains(ip) {
				return nil
			}
		}
	}
	return fmt.Errorf("endpoint %q (resolved to %v) is not in the whitelist", endpointURL, ips)
}

// connectAndDiscover creates an MCP client, starts it, discovers tools,
// and returns the invokable tool matching cfg.ToolName.
func (r *DynamicMCPRegistry) connectAndDiscover(ctx context.Context, cfg skills.UserMCPTool, timeoutMs int) (toolapi.InvokableTool, error) {
	connectCtx, connectCancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer connectCancel()

	var mcpClient *client.Client
	var err error

	switch cfg.Transport {
	case skills.TransportHTTP:
		mcpClient, err = client.NewStreamableHttpClient(cfg.EndpointURL)
	case skills.TransportSSE, "":
		mcpClient, err = client.NewSSEMCPClient(cfg.EndpointURL)
	default:
		return nil, fmt.Errorf("unsupported transport %q", cfg.Transport)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	if err = mcpClient.Start(connectCtx); err != nil {
		return nil, fmt.Errorf("failed to start MCP client: %w", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "opscaptain-dynamic",
		Version: "1.0.0",
	}
	if _, err = mcpClient.Initialize(connectCtx, initReq); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	// Discover tools via eino adapter.
	einoTools, err := e_mcp.GetTools(connectCtx, &e_mcp.Config{Cli: mcpClient})
	if err != nil {
		return nil, fmt.Errorf("failed to discover MCP tools: %w", err)
	}

	for _, t := range einoTools {
		it, ok := t.(toolapi.InvokableTool)
		if !ok {
			continue
		}
		info, _ := it.Info(ctx)
		if info != nil && info.Name == cfg.ToolName {
			return it, nil
		}
	}

	return nil, fmt.Errorf("tool %q not found on MCP server (discovered %d tools)", cfg.ToolName, len(einoTools))
}

// degradedJSON returns a JSON string indicating degraded mode.
func degradedJSON(reason string) string {
	b, _ := json.Marshal(map[string]any{
		"degraded": true,
		"reason":   reason,
	})
	return string(b)
}
