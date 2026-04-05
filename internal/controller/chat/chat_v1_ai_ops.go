package chat

import (
	"SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/service"
	"context"
	"errors"
)

var runAIOpsMultiAgent = service.RunAIOpsMultiAgent

func (c *ControllerV1) AIOps(ctx context.Context, req *v1.AIOpsReq) (res *v1.AIOpsRes, err error) {
	query := req.Query
	if query == "" {
		query = `你是一个智能的服务告警分析助手，请按照以下步骤执行分析：
1. 调用 query_prometheus_alerts 获取所有活跃告警。如果该工具返回失败或无法连接，直接跳过告警查询步骤并在报告中注明。
2. 对每个告警，调用 query_internal_docs 查找对应的处理方案。
3. 完全遵循内部文档的内容进行分析，不使用文档外的信息。
4. 涉及时间参数时，先通过 get_current_time 获取当前时间。
5. 如果某个工具不可用或返回错误，跳过该步骤并在报告中说明，不要反复重试。
6. 生成告警运维分析报告，格式如下：
# 告警分析报告
## 活跃告警清单
## 告警根因分析
## 处理方案
## 结论`
	}

	resp, detail, traceID, err := runAIOpsMultiAgent(ctx, query)
	if err != nil {
		return nil, err
	}
	if resp == "" {
		if len(detail) > 0 && detail[0] != "" {
			resp = detail[0]
		} else {
			return nil, errors.New("内部错误")
		}
	}
	res = &v1.AIOpsRes{
		TraceID: traceID,
		Result: resp,
		Detail: detail,
	}
	return res, nil
}

func (c *ControllerV1) AIOpsTrace(ctx context.Context, req *v1.AIOpsTraceReq) (res *v1.AIOpsTraceRes, err error) {
	events, detail, err := service.GetAIOpsTrace(ctx, req.TraceID)
	if err != nil {
		return nil, err
	}

	out := make([]v1.AIOpsTraceEvent, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		out = append(out, v1.AIOpsTraceEvent{
			EventID:   event.EventID,
			TaskID:    event.TaskID,
			TraceID:   event.TraceID,
			Type:      event.Type,
			Agent:     event.Agent,
			Message:   event.Message,
			Payload:   event.Payload,
			CreatedAt: event.CreatedAt,
		})
	}

	return &v1.AIOpsTraceRes{
		TraceID: req.TraceID,
		Detail:  detail,
		Events:  out,
	}, nil
}
