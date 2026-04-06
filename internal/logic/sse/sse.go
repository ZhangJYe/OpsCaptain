package sse

import (
	"context"
	"fmt"
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
	msg := fmt.Sprintf(
		"id: %d\nevent: %s\ndata: %s\n\n",
		time.Now().UnixNano(), eventType, data,
	)
	c.Request.Response.Write(msg)
	c.Request.Response.Flush()
	return true
}
