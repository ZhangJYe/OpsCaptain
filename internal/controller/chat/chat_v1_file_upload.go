package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/utility/common"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/google/uuid"
)

const (
	defaultMaxUploadSize = 20 * 1024 * 1024
	quarantineDir        = "quarantine"
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

	safeFilenameRe = regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)
)

type uploadFileRecord struct {
	SourceKind       string `json:"source_kind"`
	SourceKey        string `json:"source_key"`
	Source           string `json:"_source"`
	OriginalFilename string `json:"original_filename"`
	StoredFilename   string `json:"stored_filename"`
	ContentHash      string `json:"content_hash"`
	UploadedAt       string `json:"uploaded_at"`
	Version          int    `json:"version"`
	FileSize         int64  `json:"file_size"`
	MIMEType         string `json:"mime_type,omitempty"`

	filePath     string
	metadataPath string
}

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
	sourceKey := buildUploadSourceKey(safeName)
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

	contentHash, err := computeFileSHA256(quarantinePath)
	if err != nil {
		return nil, gerror.Wrapf(err, "计算文件哈希失败")
	}

	if !gfile.Exists(common.FileDir) {
		if err := gfile.Mkdir(common.FileDir); err != nil {
			return nil, gerror.Wrapf(err, "创建目录失败: %s", common.FileDir)
		}
	}

	existingRecords, err := listUploadRecordsBySourceKey(common.FileDir, sourceKey)
	if err != nil {
		return nil, gerror.Wrapf(err, "读取上传记录失败")
	}
	if duplicate, ok := findDuplicateUploadRecord(existingRecords, contentHash); ok {
		_ = os.Remove(quarantinePath)
		return &v1.FileUploadRes{
			FileName: safeName,
			FileSize: duplicate.FileSize,
			FileID:   duplicate.StoredFilename,
		}, nil
	}

	finalPath := filepath.Join(common.FileDir, uniqueName)
	if err := os.Rename(quarantinePath, finalPath); err != nil {
		return nil, gerror.Wrapf(err, "移动文件失败")
	}

	record := uploadFileRecord{
		SourceKind:       uploadSourceKind,
		SourceKey:        sourceKey,
		Source:           sourceKey,
		OriginalFilename: safeName,
		StoredFilename:   uniqueName,
		ContentHash:      contentHash,
		UploadedAt:       time.Now().UTC().Format(time.RFC3339),
		Version:          nextUploadVersion(existingRecords),
		FileSize:         fileInfo.Size(),
		MIMEType:         mimeType,
	}
	if err := writeUploadMetadata(finalPath, record); err != nil {
		_ = os.Remove(finalPath)
		return nil, gerror.Wrapf(err, "写入上传记录失败")
	}

	res = &v1.FileUploadRes{
		FileName: safeName,
		FileSize: fileInfo.Size(),
		FileID:   uniqueName,
	}

	err = buildIntoIndex(ctx, finalPath)
	if err != nil {
		_ = cleanupUploadRecord(record)
		return nil, gerror.Wrapf(err, "构建知识库失败")
	}
	if err := cleanupReplacedUploadRecords(existingRecords, record.StoredFilename); err != nil {
		g.Log().Warningf(ctx, "cleanup replaced upload artifacts failed: %v", err)
	}
	if len(existingRecords) > 0 {
		rag.DefaultIndexingService().SyncBM25Index(ctx)
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

func buildUploadSourceKey(safeName string) string {
	return uploadSourcePrefix + safeName
}

func computeFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func uploadMetadataPath(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path + ".metadata.json"
	}
	return path[:len(path)-len(ext)] + ".metadata.json"
}

func listUploadRecordsBySourceKey(dir string, sourceKey string) ([]uploadFileRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	records := make([]uploadFileRecord, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".metadata.json") {
			continue
		}
		record, err := readUploadRecord(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if record.SourceKind != uploadSourceKind || record.SourceKey != sourceKey {
			continue
		}
		if strings.TrimSpace(record.StoredFilename) == "" {
			continue
		}
		record.filePath = filepath.Join(dir, record.StoredFilename)
		record.metadataPath = filepath.Join(dir, entry.Name())
		records = append(records, record)
	}

	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Version != records[j].Version {
			return records[i].Version > records[j].Version
		}
		return records[i].UploadedAt > records[j].UploadedAt
	})
	return records, nil
}

func readUploadRecord(path string) (uploadFileRecord, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return uploadFileRecord{}, err
	}
	var record uploadFileRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return uploadFileRecord{}, err
	}
	record.metadataPath = path
	return record, nil
}

func findDuplicateUploadRecord(records []uploadFileRecord, contentHash string) (uploadFileRecord, bool) {
	for _, record := range records {
		if record.ContentHash != contentHash {
			continue
		}
		if strings.TrimSpace(record.StoredFilename) == "" {
			continue
		}
		if record.filePath == "" {
			record.filePath = filepath.Join(common.FileDir, record.StoredFilename)
		}
		if _, err := os.Stat(record.filePath); err == nil {
			return record, true
		}
	}
	return uploadFileRecord{}, false
}

func nextUploadVersion(records []uploadFileRecord) int {
	version := 1
	for _, record := range records {
		if record.Version >= version {
			version = record.Version + 1
		}
	}
	return version
}

func writeUploadMetadata(path string, record uploadFileRecord) error {
	record.filePath = path
	record.metadataPath = uploadMetadataPath(path)
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(record.metadataPath, body, 0o644)
}

func cleanupReplacedUploadRecords(records []uploadFileRecord, keepStoredFilename string) error {
	for _, record := range records {
		if record.StoredFilename == keepStoredFilename {
			continue
		}
		if err := cleanupUploadRecord(record); err != nil {
			return err
		}
	}
	return nil
}

func cleanupUploadRecord(record uploadFileRecord) error {
	if record.filePath == "" && strings.TrimSpace(record.StoredFilename) != "" {
		record.filePath = filepath.Join(common.FileDir, record.StoredFilename)
	}
	if record.metadataPath == "" && record.filePath != "" {
		record.metadataPath = uploadMetadataPath(record.filePath)
	}
	if record.filePath != "" {
		if err := os.Remove(record.filePath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if record.metadataPath != "" {
		if err := os.Remove(record.metadataPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
