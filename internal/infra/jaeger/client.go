package jaeger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type DependencyEdge struct {
	Parent    string `json:"parent"`
	Child     string `json:"child"`
	CallCount int64  `json:"callCount"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func NewClientWithTimeout(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) GetDependencies(ctx context.Context, lookbackHours int) ([]DependencyEdge, error) {
	if lookbackHours <= 0 {
		lookbackHours = 24
	}

	now := time.Now()
	endTs := now.UnixMilli()
	lookbackMs := int64(lookbackHours) * 3600 * 1000

	url := fmt.Sprintf("%s/api/dependencies?endTs=%d&lookback=%d", c.baseURL, endTs, lookbackMs)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create jaeger request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call jaeger dependencies API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jaeger dependencies API returned status %d: %s", resp.StatusCode, string(body))
	}

	var edges []DependencyEdge
	if err := json.NewDecoder(resp.Body).Decode(&edges); err != nil {
		return nil, fmt.Errorf("decode jaeger dependencies response: %w", err)
	}

	return edges, nil
}
