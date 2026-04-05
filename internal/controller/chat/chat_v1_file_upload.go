package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/knowledge_index_pipeline"
	loader2 "SuperBizAgent/internal/ai/loader"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/log_call_back"
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
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
	r, err := knowledge_index_pipeline.BuildKnowledgeIndexing(ctx)
	if err != nil {
		return fmt.Errorf("build knowledge indexing failed: %w", err)
	}
	loader, err := loader2.NewFileLoader(ctx)
	if err != nil {
		return err
	}
	docs, err := loader.Load(ctx, document.Source{URI: path})
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return fmt.Errorf("loader returned no documents for file: %s", path)
	}
	cli, err := client.NewMilvusClient(ctx)
	if err != nil {
		return err
	}
	expr := fmt.Sprintf(`metadata["_source"] == "%s"`, docs[0].MetaData["_source"])
	queryResult, err := cli.Query(ctx, common.MilvusCollectionName, []string{}, expr, []string{"id"})
	if err != nil {
		return err
	} else if len(queryResult) > 0 {
		var idsToDelete []string
		for _, column := range queryResult {
			if column.Name() == "id" {
				for i := 0; i < column.Len(); i++ {
					id, err := column.GetAsString(i)
					if err == nil {
						idsToDelete = append(idsToDelete, id)
					}
				}
			}
		}
		if len(idsToDelete) > 0 {
			deleteExpr := fmt.Sprintf(`id in ["%s"]`, strings.Join(idsToDelete, `","`))
			err = cli.Delete(ctx, common.MilvusCollectionName, "", deleteExpr)
			if err != nil {
				g.Log().Warningf(ctx, "delete existing data failed: %v", err)
			} else {
				g.Log().Infof(ctx, "deleted %d existing records with _source: %s", len(idsToDelete), docs[0].MetaData["_source"])
			}
		}
	}
	ids, err := r.Invoke(ctx, document.Source{URI: path}, compose.WithCallbacks(log_call_back.LogCallback(nil)))
	if err != nil {
		return fmt.Errorf("invoke index graph failed: %w", err)
	}
	g.Log().Infof(ctx, "indexing file: %s, len of parts: %d", path, len(ids))
	return nil
}
