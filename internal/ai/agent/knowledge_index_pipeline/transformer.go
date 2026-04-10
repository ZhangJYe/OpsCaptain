package knowledge_index_pipeline

import (
	"SuperBizAgent/internal/ai/embedder"
	"context"
	"unicode/utf8"

	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/markdown"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/semantic"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

const semanticSplitThreshold = 800

func newDocumentTransformer(ctx context.Context) (tfr document.Transformer, err error) {
	mdSplitter, err := markdown.NewHeaderSplitter(ctx, &markdown.HeaderConfig{
		Headers: map[string]string{
			"#":    "title",
			"##":   "subtitle",
			"###":  "section",
			"####": "subsection",
		},
		TrimHeaders: false,
		IDGenerator: func(ctx context.Context, originalID string, splitIndex int) string {
			return uuid.New().String()
		},
	})
	if err != nil {
		return nil, err
	}

	eb, err := embedder.DoubaoEmbedding(ctx)
	if err != nil {
		return nil, err
	}

	semSplitter, err := semantic.NewSplitter(ctx, &semantic.Config{
		Embedding:    eb,
		BufferSize:   1,
		MinChunkSize: 50,
		Separators:   []string{"\n\n", "\n", "。", ".", "？", "?", "！", "!", "；", ";"},
		LenFunc:      utf8.RuneCountInString,
		Percentile:   0.85,
		IDGenerator: func(ctx context.Context, originalID string, splitIndex int) string {
			return uuid.New().String()
		},
	})
	if err != nil {
		return nil, err
	}

	return &twoStageTransformer{
		stage1: mdSplitter,
		stage2: semSplitter,
	}, nil
}

type twoStageTransformer struct {
	stage1 document.Transformer
	stage2 document.Transformer
}

func (t *twoStageTransformer) Transform(ctx context.Context, docs []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	after1, err := t.stage1.Transform(ctx, docs, opts...)
	if err != nil {
		return nil, err
	}

	var small, large []*schema.Document
	for _, d := range after1 {
		if utf8.RuneCountInString(d.Content) > semanticSplitThreshold {
			large = append(large, d)
		} else {
			small = append(small, d)
		}
	}

	if len(large) == 0 {
		return after1, nil
	}

	after2, err := t.stage2.Transform(ctx, large, opts...)
	if err != nil {
		return after1, nil
	}

	result := make([]*schema.Document, 0, len(small)+len(after2))
	result = append(result, small...)
	result = append(result, after2...)
	return result, nil
}
