package loader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

func NewFileLoader(ctx context.Context) (ldr document.Loader, err error) {
	config := &file.FileLoaderConfig{}
	inner, err := file.NewFileLoader(ctx, config)
	if err != nil {
		return nil, err
	}
	return &metadataSidecarLoader{inner: inner}, nil
}

type metadataSidecarLoader struct {
	inner document.Loader
}

func (l *metadataSidecarLoader) Load(ctx context.Context, src document.Source, opts ...document.LoaderOption) ([]*schema.Document, error) {
	docs, err := l.inner.Load(ctx, src, opts...)
	if err != nil {
		return nil, err
	}

	sidecar, err := loadSidecarMetadata(src.URI)
	if err != nil {
		return nil, err
	}
	if len(sidecar) == 0 {
		return docs, nil
	}

	for _, doc := range docs {
		if doc == nil {
			continue
		}
		doc.MetaData = mergeMetadata(doc.MetaData, sidecar)
	}
	return docs, nil
}

func loadSidecarMetadata(path string) (map[string]any, error) {
	if path == "" {
		return nil, nil
	}
	sidecarPath := stringsTrimExt(path) + ".metadata.json"
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read metadata sidecar %s: %w", sidecarPath, err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("decode metadata sidecar %s: %w", sidecarPath, err)
	}
	return metadata, nil
}

func mergeMetadata(existing, sidecar map[string]any) map[string]any {
	if len(sidecar) == 0 {
		return existing
	}
	out := make(map[string]any, len(existing)+len(sidecar))
	for k, v := range sidecar {
		out[k] = v
	}
	for k, v := range existing {
		if _, ok := out[k]; ok {
			continue
		}
		out[k] = v
	}
	return out
}

func stringsTrimExt(path string) string {
	ext := filepath.Ext(path)
	return path[:len(path)-len(ext)]
}
