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
	TokenAudit(ctx context.Context, req *v1.TokenAuditReq) (res *v1.TokenAuditRes, err error)
	ApprovalRequests(ctx context.Context, req *v1.ApprovalRequestsReq) (res *v1.ApprovalRequestsRes, err error)
	ApproveApprovalRequest(ctx context.Context, req *v1.ApprovalActionReq) (res *v1.AIOpsRes, err error)
	RejectApprovalRequest(ctx context.Context, req *v1.ApprovalRejectReq) (res *v1.ApprovalRequestItem, err error)
	MemoryList(ctx context.Context, req *v1.MemoryListReq) (res *v1.MemoryListRes, err error)
	MemoryAction(ctx context.Context, req *v1.MemoryActionReq) (res *v1.MemoryActionRes, err error)
	MemoryPromote(ctx context.Context, req *v1.MemoryPromoteReq) (res *v1.MemoryActionRes, err error)
}
