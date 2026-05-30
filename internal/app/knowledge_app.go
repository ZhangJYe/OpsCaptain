package app

import (
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/internal/infra/filestore"
	"SuperBizAgent/utility/common"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultMaxUploadSize = 20 * 1024 * 1024
	uploadSourceKind     = "chat_upload"
	uploadSourcePrefix   = "upload://"
)

var (
	allowedExtensions = map[string]bool{
		".md": true, ".txt": true, ".pdf": true,
		".doc": true, ".docx": true, ".csv": true,
		".json": true, ".yaml": true, ".yml": true,
	}
	allowedMIMEPrefixes = []string{
		"text/",
		"application/pdf",
		"application/json",
		"application/vnd.openxmlformats",
		"application/msword",
		"application/x-yaml",
	}
)

// UploadInput is the application-layer input for a file upload.
type UploadInput struct {
	Filename string
	MIMEType string
	Size     int64
	Content  []byte
}

// UploadResult is the application-layer output for a file upload.
type UploadResult struct {
	FileName string
	FileID   string
	FileSize int64
	Status   string
}

// UploadStatusResult is the application-layer output for an upload status query.
type UploadStatusResult struct {
	FileID string
	Status string
}

type KnowledgeApp struct {
	uploadStore filestore.UploadStore
	indexSource func(context.Context, string) error
}

func NewKnowledgeApp(stores ...filestore.UploadStore) *KnowledgeApp {
	store := filestore.UploadStore(filestore.NewLocalUploadStore(common.FileDir))
	if len(stores) > 0 && stores[0] != nil {
		store = stores[0]
	}
	return &KnowledgeApp{
		uploadStore: store,
		indexSource: buildIntoIndex,
	}
}

func (k *KnowledgeApp) HandleUpload(ctx context.Context, input *UploadInput) (*UploadResult, error) {
	ext := strings.ToLower(filepath.Ext(input.Filename))
	if !allowedExtensions[ext] {
		return nil, fmt.Errorf("不支持的文件类型: %s, 允许: %v", ext, allowedExtensionList())
	}
	if !isAllowedMIME(input.MIMEType) {
		return nil, fmt.Errorf("不支持的MIME类型: %s", input.MIMEType)
	}
	maxSize := getMaxUploadSize(ctx)
	if input.Size > maxSize {
		return nil, fmt.Errorf("文件过大: %d bytes, 最大允许: %d MB", input.Size, maxSize/(1024*1024))
	}

	saved, err := k.uploadStore.SaveUpload(ctx, filestore.UploadSaveInput{
		Filename:     input.Filename,
		MIMEType:     input.MIMEType,
		Size:         input.Size,
		Content:      input.Content,
		SourceKind:   uploadSourceKind,
		SourcePrefix: uploadSourcePrefix,
	})
	if err != nil {
		return nil, err
	}
	if saved.Duplicate {
		return &UploadResult{
			FileName: saved.FileName,
			FileSize: saved.FileSize,
			FileID:   saved.FileID,
			Status:   saved.Status,
		}, nil
	}

	go func() {
		indexCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		if err := k.indexSource(indexCtx, saved.FilePath); err != nil {
			g.Log().Warningf(indexCtx, "async indexing failed for %s: %v", saved.FilePath, err)
			if markErr := k.uploadStore.MarkUploadStatus(indexCtx, saved.FileID, "failed"); markErr != nil {
				g.Log().Warningf(indexCtx, "mark upload indexing failed status failed for %s: %v", saved.FileID, markErr)
			}
			return
		}
		if err := k.uploadStore.MarkUploadStatus(indexCtx, saved.FileID, "ready"); err != nil {
			g.Log().Warningf(indexCtx, "mark upload indexing ready status failed for %s: %v", saved.FileID, err)
		}
		if err := k.uploadStore.CleanupReplacedUploads(indexCtx, uploadSourceKind, saved.SourceKey, saved.FileID); err != nil {
			g.Log().Warningf(indexCtx, "cleanup replaced upload artifacts failed: %v", err)
		}
	}()

	return &UploadResult{
		FileName: saved.FileName,
		FileSize: saved.FileSize,
		FileID:   saved.FileID,
		Status:   saved.Status,
	}, nil
}

func (k *KnowledgeApp) HandleUploadStatus(ctx context.Context, fileID string) (*UploadStatusResult, error) {
	status, err := k.uploadStore.UploadStatus(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("文件记录不存在: %s", fileID)
	}
	return &UploadStatusResult{FileID: fileID, Status: status}, nil
}

func IsAllowedExtension(ext string) bool {
	return allowedExtensions[ext]
}

func AllowedExtensions() []string {
	return allowedExtensionList()
}

func MaxUploadSize(ctx context.Context) int64 {
	return getMaxUploadSize(ctx)
}

func getMaxUploadSize(ctx context.Context) int64 {
	v, err := g.Cfg().Get(ctx, "upload.max_size_mb")
	if err == nil && v.Int64() > 0 {
		return v.Int64() * 1024 * 1024
	}
	return defaultMaxUploadSize
}

func isAllowedMIME(mimeType string) bool {
	mimeType = strings.ToLower(mimeType)
	for _, prefix := range allowedMIMEPrefixes {
		if strings.HasPrefix(mimeType, prefix) {
			return true
		}
	}
	return false
}

func allowedExtensionList() []string {
	list := make([]string, 0, len(allowedExtensions))
	for ext := range allowedExtensions {
		list = append(list, ext)
	}
	return list
}

func buildIntoIndex(ctx context.Context, path string) error {
	summary, err := rag.DefaultIndexingService().IndexSource(ctx, path)
	if err != nil {
		return err
	}
	g.Log().Infof(ctx, "indexing file: %s, deleted=%d, len of parts: %d", summary.SourcePath, summary.DeletedExisting, len(summary.ChunkIDs))
	return nil
}
