package sse

import (
	"context"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/utility/middleware"

	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
)

type Client struct {
	Id      string
	Request *ghttp.Request
}

type Service struct {
	clients *gmap.StrAnyMap
}

func New() *Service {
	return &Service{
		clients: gmap.NewStrAnyMap(true),
	}
}

func (s *Service) Create(ctx context.Context, r *ghttp.Request) (*Client, error) {
	r.Response.Header().Set("Content-Type", "text/event-stream")
	r.Response.Header().Set("Cache-Control", "no-cache")
	r.Response.Header().Set("Connection", "keep-alive")
	if origin, ok := middleware.ResolveAllowedOrigin(ctx, r.GetHeader("Origin")); ok {
		r.Response.Header().Set("Access-Control-Allow-Origin", origin)
		r.Response.Header().Set("Vary", "Origin")
	}
	clientId := r.Get("client_id", guid.S()).String()
	client := &Client{
		Id:      clientId,
		Request: r,
	}
	r.Response.Writefln("id: %s", clientId)
	r.Response.Writefln("event: connected")
	r.Response.Writefln("data: {\"status\": \"connected\", \"client_id\": \"%s\"}\n", clientId)
	r.Response.Flush()
	return client, nil
}

func (c *Client) SendToClient(eventType, data string) bool {
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("id: %d\n", time.Now().UnixNano()))
	b.WriteString(fmt.Sprintf("event: %s\n", eventType))
	for _, line := range lines {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	msg := b.String()
	c.Request.Response.Write(msg)
	c.Request.Response.Flush()
	return true
}
