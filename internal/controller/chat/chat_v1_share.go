package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"context"
)

func (c *ControllerV1) ShareCreate(ctx context.Context, req *v1.ShareCreateReq) (res *v1.ShareCreateRes, err error) {
	link, err := c.shareStore.Create(req.SessionID, "", req.TTLHours)
	if err != nil {
		return &v1.ShareCreateRes{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	return &v1.ShareCreateRes{
		Success: true,
		Share:   link,
		URL:     "/share/" + link.ID,
	}, nil
}

func (c *ControllerV1) ShareGet(ctx context.Context, req *v1.ShareGetReq) (res *v1.ShareGetRes, err error) {
	link, ok := c.shareStore.Get(req.ShareID)
	if !ok {
		return &v1.ShareGetRes{
			Success: false,
			Error:   "share link not found or expired",
		}, nil
	}
	return &v1.ShareGetRes{
		Success: true,
		Session: map[string]interface{}{
			"session_id": link.SessionID,
			"created_by": link.CreatedBy,
			"created_at": link.CreatedAt,
			"expires_at": link.ExpiresAt,
		},
	}, nil
}

func (c *ControllerV1) ShareRevoke(ctx context.Context, req *v1.ShareRevokeReq) (res *v1.ShareRevokeRes, err error) {
	if err := c.shareStore.Revoke(req.ShareID); err != nil {
		return &v1.ShareRevokeRes{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	return &v1.ShareRevokeRes{
		Success: true,
	}, nil
}
