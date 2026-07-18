package experts

import "encoding/json"

type ArgBuilder interface {
	Build(args string) (string, error)
}

type QueryArgBuilder struct{}

func (b *QueryArgBuilder) Build(args string) (string, error) {
	bytes, err := json.Marshal(map[string]string{"query": args})
	return string(bytes), err
}

type RawArgBuilder struct{}

func (b *RawArgBuilder) Build(args string) (string, error) {
	return args, nil
}

func GetArgBuilder(toolName string) ArgBuilder {
	switch toolName {
	case "query_internal_docs", "query_logs", "query_prometheus_alerts":
		return &QueryArgBuilder{}
	case "get_current_time":
		return &RawArgBuilder{}
	default:
		return &QueryArgBuilder{}
	}
}
