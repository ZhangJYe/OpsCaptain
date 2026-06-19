package contextengine

import "context"

// CMDBEnricher auto-injects CMDB context items into the context package.
type CMDBEnricher interface {
	Enrich(ctx context.Context, query string) []ContextItem
}
