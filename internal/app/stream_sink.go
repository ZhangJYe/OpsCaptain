package app

type StreamSink interface {
	SendMeta(event ChatStreamMetaEvent)
	SendText(text string)
	SendDetails(details []string)
	SendEvent(eventType, data string)
}

// ChatStreamMetaEvent is the structured metadata sent at stream start.
type ChatStreamMetaEvent struct {
	Mode              string   `json:"mode"`
	TraceID           string   `json:"trace_id"`
	Detail            []string `json:"detail"`
	Degraded          bool     `json:"degraded"`
	DegradationReason string   `json:"degradation_reason"`
}

// ChatStreamResult is the application-layer output of a streaming chat request.
type ChatStreamResult struct {
	FullResponse      string
	TraceID           string
	Degraded          bool
	DegradationReason string
}
