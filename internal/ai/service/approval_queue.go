package service

import (
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/common"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

const (
	defaultApprovalKeyPrefix = "opscaptionai:approval"
	defaultApprovalTTL       = 24 * time.Hour
)

type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusRejected ApprovalStatus = "rejected"
	ApprovalStatusExecuted ApprovalStatus = "executed"
)

type ApprovalRequest struct {
	ID            string         `json:"id"`
	Query         string         `json:"query"`
	Reason        string         `json:"reason"`
	Status        ApprovalStatus `json:"status"`
	SessionID     string         `json:"session_id,omitempty"`
	UserID        string         `json:"user_id,omitempty"`
	RequestedBy   string         `json:"requested_by,omitempty"`
	ReviewedBy    string         `json:"reviewed_by,omitempty"`
	ReviewReason  string         `json:"review_reason,omitempty"`
	ExecutionPlan []string       `json:"execution_plan,omitempty"`
	ResultTraceID string         `json:"result_trace_id,omitempty"`
	CreatedAt     int64          `json:"created_at"`
	UpdatedAt     int64          `json:"updated_at"`
	ApprovedAt    int64          `json:"approved_at,omitempty"`
	RejectedAt    int64          `json:"rejected_at,omitempty"`
	ExecutionAt   int64          `json:"execution_at,omitempty"`
}

type ApprovalQueue struct{}

func NewApprovalQueue() *ApprovalQueue {
	return &ApprovalQueue{}
}

func (q *ApprovalQueue) Enqueue(ctx context.Context, query, reason string, plan []string) (*ApprovalRequest, error) {
	if !approvalQueueEnabled(ctx) {
		return nil, fmt.Errorf("approval queue is not enabled")
	}

	now := time.Now().Unix()
	request := &ApprovalRequest{
		ID:            uuid.NewString(),
		Query:         strings.TrimSpace(query),
		Reason:        strings.TrimSpace(reason),
		Status:        ApprovalStatusPending,
		ExecutionPlan: append([]string{}, plan...),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if sessionID, ok := ctx.Value(consts.CtxKeySessionID).(string); ok {
		request.SessionID = strings.TrimSpace(sessionID)
	}
	if userID, ok := ctx.Value(consts.CtxKeyUserID).(string); ok {
		request.UserID = strings.TrimSpace(userID)
		request.RequestedBy = strings.TrimSpace(userID)
	}

	if err := q.save(ctx, request, ApprovalStatusPending); err != nil {
		return nil, err
	}
	return request, nil
}

func (q *ApprovalQueue) Get(ctx context.Context, requestID string) (*ApprovalRequest, error) {
	value, err := g.Redis().Do(ctx, "GET", q.requestKey(requestID))
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(value.String())
	if raw == "" {
		return nil, fmt.Errorf("approval request %s not found", requestID)
	}

	var request ApprovalRequest
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return nil, err
	}
	return &request, nil
}

func (q *ApprovalQueue) List(ctx context.Context, status ApprovalStatus) ([]ApprovalRequest, error) {
	if !approvalQueueEnabled(ctx) {
		return nil, nil
	}

	if status == "" {
		status = ApprovalStatusPending
	}
	values, err := g.Redis().Do(ctx, "SMEMBERS", q.statusKey(status))
	if err != nil {
		return nil, err
	}
	ids := values.Strings()
	if len(ids) == 0 {
		return nil, nil
	}

	out := make([]ApprovalRequest, 0, len(ids))
	for _, id := range ids {
		request, err := q.Get(ctx, id)
		if err != nil || request == nil {
			continue
		}
		out = append(out, *request)
	}
	return out, nil
}

func (q *ApprovalQueue) Approve(ctx context.Context, requestID, reviewer string) (*ApprovalRequest, error) {
	request, err := q.Get(ctx, requestID)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	request.Status = ApprovalStatusApproved
	request.ReviewedBy = strings.TrimSpace(reviewer)
	request.ApprovedAt = now
	request.UpdatedAt = now
	if err := q.save(ctx, request, ApprovalStatusPending); err != nil {
		return nil, err
	}
	return request, nil
}

func (q *ApprovalQueue) Reject(ctx context.Context, requestID, reviewer, reviewReason string) (*ApprovalRequest, error) {
	request, err := q.Get(ctx, requestID)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	request.Status = ApprovalStatusRejected
	request.ReviewedBy = strings.TrimSpace(reviewer)
	request.ReviewReason = strings.TrimSpace(reviewReason)
	request.RejectedAt = now
	request.UpdatedAt = now
	if err := q.save(ctx, request, ApprovalStatusPending); err != nil {
		return nil, err
	}
	return request, nil
}

func (q *ApprovalQueue) MarkExecuted(ctx context.Context, requestID, traceID string) error {
	request, err := q.Get(ctx, requestID)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	request.Status = ApprovalStatusExecuted
	request.ResultTraceID = strings.TrimSpace(traceID)
	request.ExecutionAt = now
	request.UpdatedAt = now
	return q.save(ctx, request, ApprovalStatusApproved)
}

func (q *ApprovalQueue) save(ctx context.Context, request *ApprovalRequest, previous ApprovalStatus) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}

	key := q.requestKey(request.ID)
	if _, err := g.Redis().Do(ctx, "SETEX", key, int(approvalRequestTTL(ctx).Seconds()), string(payload)); err != nil {
		return err
	}
	if previous != "" && previous != request.Status {
		if _, err := g.Redis().Do(ctx, "SREM", q.statusKey(previous), request.ID); err != nil {
			return err
		}
	}
	_, err = g.Redis().Do(ctx, "SADD", q.statusKey(request.Status), request.ID)
	return err
}

func (q *ApprovalQueue) requestKey(id string) string {
	return fmt.Sprintf("%s:request:%s", approvalKeyPrefix(), strings.TrimSpace(id))
}

func (q *ApprovalQueue) statusKey(status ApprovalStatus) string {
	return fmt.Sprintf("%s:status:%s", approvalKeyPrefix(), status)
}

func approvalQueueEnabled(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "approval.enabled")
	if err == nil && strings.TrimSpace(v.String()) != "" && !v.Bool() {
		return false
	}
	addr, err := g.Cfg().Get(ctx, "redis.default.address")
	if err != nil {
		return false
	}
	_, ok := common.ResolveOptionalEnv(addr.String())
	return ok
}

func approvalRequestTTL(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "approval.request_ttl_seconds")
	if err != nil || v.Int64() <= 0 {
		return defaultApprovalTTL
	}
	return time.Duration(v.Int64()) * time.Second
}

func approvalKeyPrefix() string {
	ctx := context.Background()
	v, err := g.Cfg().Get(ctx, "approval.redis_key_prefix")
	if err != nil || strings.TrimSpace(v.String()) == "" {
		return defaultApprovalKeyPrefix
	}
	return strings.TrimSpace(v.String())
}
