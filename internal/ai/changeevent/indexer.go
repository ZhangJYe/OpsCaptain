package changeevent

import (
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/rag"
	"context"
	"fmt"
	"strings"
	"time"
)

// ChangeRAGIndexer 将变更事件摘要索引到 RAG 知识库（语义辅助检索）。
// 这不是主存储——主链路是结构化事件存储 + 时间窗口精确关联。
// RAG 索引用于语义模糊查询，如"最近和 nginx 5xx 相关的发布变更"。
type ChangeRAGIndexer struct {
	sourcePrefix string
}

// NewChangeRAGIndexer 创建变更事件 RAG 索引器。
func NewChangeRAGIndexer(sourcePrefix string) *ChangeRAGIndexer {
	if sourcePrefix == "" {
		sourcePrefix = "change_event"
	}
	return &ChangeRAGIndexer{sourcePrefix: sourcePrefix}
}

// Name 实现 ChangeEventHandler 接口。
func (idx *ChangeRAGIndexer) Name() string {
	return "change_rag_indexer"
}

// Handle 将变更事件索引为 RAG 可检索的文档。
func (idx *ChangeRAGIndexer) Handle(ctx context.Context, event *protocol.ChangeEvent) error {
	content := changeEventToDocument(event, idx.sourcePrefix)
	if content == "" {
		return nil
	}

	// 索引到 BM25（词法检索）
	meta := changeEventMetadata(event, idx.sourcePrefix)
	strMeta := make(map[string]string)
	for k, v := range meta {
		strMeta[k] = fmt.Sprintf("%v", v)
	}

	bm25 := rag.SharedBM25Index()
	if bm25 != nil {
		bm25.AddDocument(idx.sourcePrefix+":"+event.EventID, content, strMeta)
	}

	return nil
}

// changeEventToDocument 将变更事件转换为 RAG 文档格式。
func changeEventToDocument(event *protocol.ChangeEvent, sourcePrefix string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# 变更事件: %s %s\n\n", event.Service, event.EventType))
	b.WriteString(fmt.Sprintf("- **时间**: %s\n", event.StartedAt.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("- **来源**: %s\n", event.Source))
	b.WriteString(fmt.Sprintf("- **类型**: %s\n", event.EventType))
	b.WriteString(fmt.Sprintf("- **服务**: %s\n", event.Service))
	if event.Env != "" {
		b.WriteString(fmt.Sprintf("- **环境**: %s\n", event.Env))
	}
	if event.Cluster != "" {
		b.WriteString(fmt.Sprintf("- **集群**: %s\n", event.Cluster))
	}
	if event.Operator != "" {
		b.WriteString(fmt.Sprintf("- **操作者**: %s\n", event.Operator))
	}
	b.WriteString(fmt.Sprintf("- **风险等级**: %s\n", event.RiskLevel))

	b.WriteString(fmt.Sprintf("\n## 描述\n%s\n", event.Summary))

	if event.Before != nil || event.After != nil {
		b.WriteString("\n## 变更详情\n")
		if event.Before != nil {
			b.WriteString(fmt.Sprintf("- **Before**: %v\n", event.Before))
		}
		if event.After != nil {
			b.WriteString(fmt.Sprintf("- **After**: %v\n", event.After))
		}
	}
	if event.Diff != "" {
		b.WriteString(fmt.Sprintf("- **Diff**: %s\n", event.Diff))
	}

	return b.String()
}

// changeEventMetadata 构建 RAG 文档元数据。
func changeEventMetadata(event *protocol.ChangeEvent, sourcePrefix string) map[string]any {
	meta := map[string]any{
		"_source":        sourcePrefix + ":" + event.EventID,
		"source_type":    "change_event",
		"event_type":     event.EventType,
		"service":        event.Service,
		"service_tokens": tokenizeServiceName(event.Service),
		"env":            event.Env,
		"cluster":        event.Cluster,
		"risk_level":     event.RiskLevel,
		"created_at":     event.CreatedAt.Format(time.RFC3339),
	}
	return meta
}

// tokenizeServiceName 将服务名拆分为可检索的 token。
// 例: "user-service" → ["user", "service"]
func tokenizeServiceName(name string) []string {
	name = strings.ToLower(name)
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	var tokens []string
	seen := make(map[string]bool)
	for _, p := range parts {
		if len(p) >= 2 && !seen[p] {
			seen[p] = true
			tokens = append(tokens, p)
		}
	}
	return tokens
}
