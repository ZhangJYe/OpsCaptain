package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/app"
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

// ChangeEventCreate 接收变更事件。
func (c *ControllerV1) ChangeEventCreate(ctx context.Context, req *v1.ChangeEventCreateReq) (res *v1.ChangeEventCreateRes, err error) {
	input := &app.ChangeEventInput{
		Source:     req.Source,
		EventType:  req.EventType,
		Service:    req.Service,
		Env:        req.Env,
		Namespace:  req.Namespace,
		Cluster:    req.Cluster,
		Summary:    req.Summary,
		Operator:   req.Operator,
		Before:     req.Before,
		After:      req.After,
		Diff:       req.Diff,
		RiskLevel:  req.RiskLevel,
		DedupeKey:  req.DedupeKey,
		RawPayload: req.RawPayload,
		Metadata:   req.Metadata,
	}

	// 解析 started_at
	if req.StartedAt != "" {
		t, parseErr := time.Parse(time.RFC3339, req.StartedAt)
		if parseErr != nil {
			return nil, gerror.Newf("invalid started_at format: %v", parseErr)
		}
		input.StartedAt = t
	}

	eventID, accepted, err := c.changeEventApp.IngestEvent(ctx, input)
	if err != nil {
		return nil, err
	}

	return &v1.ChangeEventCreateRes{
		EventID:  eventID,
		Accepted: accepted,
	}, nil
}

// ChangeEventList 结构化查询变更事件。
func (c *ControllerV1) ChangeEventList(ctx context.Context, req *v1.ChangeEventListReq) (res *v1.ChangeEventListRes, err error) {
	filter := app.QueryFilter{
		Env:       req.Env,
		EventType: req.EventType,
		RiskLevel: req.RiskLevel,
		Limit:     req.Limit,
	}

	if req.Service != "" {
		filter.Services = []string{req.Service}
	}

	if req.Since != "" {
		t, parseErr := time.Parse(time.RFC3339, req.Since)
		if parseErr != nil {
			return nil, gerror.Newf("invalid since format: %v", parseErr)
		}
		filter.Since = &t
	}
	if req.Until != "" {
		t, parseErr := time.Parse(time.RFC3339, req.Until)
		if parseErr != nil {
			return nil, gerror.Newf("invalid until format: %v", parseErr)
		}
		filter.Until = &t
	}

	events, err := c.changeEventApp.QueryEvents(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]v1.ChangeEventItem, 0, len(events))
	for _, e := range events {
		items = append(items, toChangeEventItem(e))
	}

	return &v1.ChangeEventListRes{
		Items: items,
		Total: len(items),
	}, nil
}

// ChangeEventGet 获取单个变更事件详情。
func (c *ControllerV1) ChangeEventGet(ctx context.Context, req *v1.ChangeEventGetReq) (res *v1.ChangeEventGetRes, err error) {
	event, err := c.changeEventApp.GetEvent(ctx, req.EventID)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, gerror.Newf("change event %q not found", req.EventID)
	}
	return &v1.ChangeEventGetRes{
		Item: toChangeEventItem(event),
	}, nil
}

// ChangeEventWebhook 从 webhook 接入变更事件。
func (c *ControllerV1) ChangeEventWebhook(ctx context.Context, req *v1.ChangeEventWebhookReq) (res *v1.ChangeEventWebhookRes, err error) {
	r := g.RequestFromCtx(ctx)
	body := r.GetBody()

	eventID, accepted, err := c.changeEventApp.IngestFromWebhook(ctx, req.Source, r.Request.Header, body)
	if err != nil {
		return nil, err
	}

	return &v1.ChangeEventWebhookRes{
		EventID:  eventID,
		Accepted: accepted,
	}, nil
}

// ChangeEventStream SSE 订阅变更事件。
func (c *ControllerV1) ChangeEventStream(ctx context.Context, req *v1.ChangeEventStreamReq) (res *v1.ChangeEventStreamRes, err error) {
	client, err := c.service.Create(ctx, g.RequestFromCtx(ctx))
	if err != nil {
		return nil, err
	}
	_, ch, cancel, err := c.changeEventApp.Subscribe(ctx, app.SubscribeFilter{
		Services: serviceFilter(req.Service),
		Env:      req.Env,
	})
	if err != nil {
		return nil, err
	}
	defer cancel()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return &v1.ChangeEventStreamRes{}, nil
		case event, ok := <-ch:
			if !ok {
				return &v1.ChangeEventStreamRes{}, nil
			}
			payload, marshalErr := json.Marshal(toChangeEventItem(event))
			if marshalErr != nil {
				continue
			}
			client.SendToClient("change_event", string(payload))
		case <-ticker.C:
			client.SendToClient("heartbeat", `{"ok":true}`)
		}
	}
}

func serviceFilter(service string) []string {
	if service == "" {
		return nil
	}
	return []string{service}
}

// toChangeEventItem 将 protocol.ChangeEvent 转换为 API 响应格式。
func toChangeEventItem(e *protocol.ChangeEvent) v1.ChangeEventItem {
	return v1.ChangeEventItem{
		EventID:         e.EventID,
		Source:          e.Source,
		EventType:       e.EventType,
		Service:         e.Service,
		Env:             e.Env,
		Namespace:       e.Namespace,
		Cluster:         e.Cluster,
		Summary:         e.Summary,
		Before:          e.Before,
		After:           e.After,
		Diff:            e.Diff,
		RiskLevel:       e.RiskLevel,
		Operator:        e.Operator,
		StartedAt:       e.StartedAt.Format(time.RFC3339),
		FinishedAt:      e.FinishedAt.Format(time.RFC3339),
		CorrelationKeys: e.CorrelationKeys,
	}
}
