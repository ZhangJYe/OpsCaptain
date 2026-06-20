package collaboration

type ShareLink struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	CreatedBy string `json:"created_by"`
	ExpiresAt int64  `json:"expires_at"`
	CreatedAt int64  `json:"created_at"`
}
