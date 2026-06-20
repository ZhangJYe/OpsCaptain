package feedback

type FeedbackRating string

const (
	RatingHelpful    FeedbackRating = "helpful"
	RatingNotHelpful FeedbackRating = "not_helpful"
)

type FeedbackEntry struct {
	ID        string         `json:"id"`
	SessionID string         `json:"session_id"`
	Query     string         `json:"query"`
	Rating    FeedbackRating `json:"rating"`
	Comment   string         `json:"comment,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
	CreatedAt int64          `json:"created_at"`
}

type FeedbackStats struct {
	Total      int     `json:"total"`
	Helpful    int     `json:"helpful"`
	NotHelpful int     `json:"not_helpful"`
	Score      float64 `json:"score"`
}
