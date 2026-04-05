package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/utility/common"
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/google/uuid"
)

const (
	defaultMaxUploadSize = 20 * 1024 * 1024
	quarantineDir        = "quarantine"
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

	safeFilenameRe = regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)
)

func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "..", "")
	name = safeFilenameRe.ReplaceAllString(name, "_")
	if name == "" || name == "." {
		name = "unnamed"
	}
	return name
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

func (c *ControllerV1) FileUpload(ctx context.Context, req *v1.FileUploadReq) (res *v1.FileUploadRes, err error) {
	r := g.RequestFromCtx(ctx)
	uploadFile := r.GetUploadFile("file")
	if uploadFile == nil {
		return nil, gerror.New("请上传文件")
	}

	ext := strings.ToLower(filepath.Ext(uploadFile.Filename))
	if !allowedExtensions[ext] {
		return nil, gerror.Newf("不支持的文件类型: %s, 允许: %v", ext, allowedExtensionList())
	}

	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = uploadFile.FileHeader.Header.Get("Content-Type")
	}
	if !isAllowedMIME(mimeType) {
		return nil, gerror.Newf("不支持的MIME类型: %s", mimeType)
	}

	maxSize := getMaxUploadSize(ctx)
	if uploadFile.Size > maxSize {
		return nil, gerror.Newf("文件过大: %d bytes, 最大允许: %d MB", uploadFile.Size, maxSize/(1024*1024))
	}

	qDir := filepath.Join(common.FileDir, quarantineDir)
	if !gfile.Exists(qDir) {
		if err := gfile.Mkdir(qDir); err != nil {
			return nil, gerror.Wrapf(err, "创建隔离目录失败: %s", qDir)
		}
	}

	safeName := sanitizeFilename(uploadFile.Filename)
	uniqueName := fmt.Sprintf("%s_%s", uuid.New().String()[:8], safeName)

	uploadFile.Filename = uniqueName
	_, err = uploadFile.Save(qDir, false)
	if err != nil {
		return nil, gerror.Wrapf(err, "保存文件失败")
	}

	quarantinePath := filepath.Join(qDir, uniqueName)
	fileInfo, err := os.Stat(quarantinePath)
	if err != nil {
		return nil, gerror.Wrapf(err, "获取文件信息失败")
	}

	if !gfile.Exists(common.FileDir) {
		if err := gfile.Mkdir(common.FileDir); err != nil {
			return nil, gerror.Wrapf(err, "创建目录失败: %s", common.FileDir)
		}
	}

	finalPath := filepath.Join(common.FileDir, uniqueName)
	if err := os.Rename(quarantinePath, finalPath); err != nil {
		return nil, gerror.Wrapf(err, "移动文件失败")
	}

	res = &v1.FileUploadRes{
		FileName: safeName,
		FileSize: fileInfo.Size(),
		FileID:   uniqueName,
	}

	err = buildIntoIndex(ctx, finalPath)
	if err != nil {
		return nil, gerror.Wrapf(err, "构建知识库失败")
	}
	return res, nil
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
