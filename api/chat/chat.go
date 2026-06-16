// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package chat

import (
	"context"

	"SuperBizAgent/api/chat/v1"
)

type IChatV1 interface {
	Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error)
	ChatSubmit(ctx context.Context, req *v1.ChatSubmitReq) (res *v1.ChatSubmitRes, err error)
	ChatTask(ctx context.Context, req *v1.ChatTaskReq) (res *v1.ChatTaskRes, err error)
	ChatStream(ctx context.Context, req *v1.ChatStreamReq) (res *v1.ChatStreamRes, err error)
	FileUpload(ctx context.Context, req *v1.FileUploadReq) (res *v1.FileUploadRes, err error)
	AIOps(ctx context.Context, req *v1.AIOpsReq) (res *v1.AIOpsRes, err error)
	AIOpsTrace(ctx context.Context, req *v1.AIOpsTraceReq) (res *v1.AIOpsTraceRes, err error)
	AIOpsIncidentCreate(ctx context.Context, req *v1.AIOpsIncidentCreateReq) (res *v1.AIOpsIncidentRes, err error)
	AIOpsIncidentTurn(ctx context.Context, req *v1.AIOpsIncidentTurnReq) (res *v1.AIOpsIncidentRes, err error)
	AIOpsIncidentList(ctx context.Context, req *v1.AIOpsIncidentListReq) (res *v1.AIOpsIncidentListRes, err error)
	AIOpsIncidentGet(ctx context.Context, req *v1.AIOpsIncidentGetReq) (res *v1.AIOpsIncidentRes, err error)
	AIOpsIncidentEvents(ctx context.Context, req *v1.AIOpsIncidentEventsReq) (res *v1.AIOpsIncidentEventsRes, err error)
	TokenAudit(ctx context.Context, req *v1.TokenAuditReq) (res *v1.TokenAuditRes, err error)
	ApprovalRequests(ctx context.Context, req *v1.ApprovalRequestsReq) (res *v1.ApprovalRequestsRes, err error)
	ApproveApprovalRequest(ctx context.Context, req *v1.ApprovalActionReq) (res *v1.AIOpsRes, err error)
	RejectApprovalRequest(ctx context.Context, req *v1.ApprovalRejectReq) (res *v1.ApprovalRequestItem, err error)
	MemoryList(ctx context.Context, req *v1.MemoryListReq) (res *v1.MemoryListRes, err error)
	MemoryAction(ctx context.Context, req *v1.MemoryActionReq) (res *v1.MemoryActionRes, err error)
	MemoryPromote(ctx context.Context, req *v1.MemoryPromoteReq) (res *v1.MemoryActionRes, err error)
	ChangeEventCreate(ctx context.Context, req *v1.ChangeEventCreateReq) (res *v1.ChangeEventCreateRes, err error)
	ChangeEventList(ctx context.Context, req *v1.ChangeEventListReq) (res *v1.ChangeEventListRes, err error)
	ChangeEventGet(ctx context.Context, req *v1.ChangeEventGetReq) (res *v1.ChangeEventGetRes, err error)
	ChangeEventStream(ctx context.Context, req *v1.ChangeEventStreamReq) (res *v1.ChangeEventStreamRes, err error)
	ChangeEventWebhook(ctx context.Context, req *v1.ChangeEventWebhookReq) (res *v1.ChangeEventWebhookRes, err error)
	NotificationConfig(ctx context.Context, req *v1.NotificationConfigReq) (res *v1.NotificationConfigRes, err error)
	NotificationTest(ctx context.Context, req *v1.NotificationTestReq) (res *v1.NotificationTestRes, err error)
}
