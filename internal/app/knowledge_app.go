package app

import (
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/utility/common"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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
	IndexStatus      string `json:"index_status"`

	filePath     string
	metadataPath string
}

// KnowledgeApp orchestrates knowledge base operations: upload, indexing, status.
type KnowledgeApp struct{}

// NewKnowledgeApp creates a KnowledgeApp.
func NewKnowledgeApp() *KnowledgeApp {
	return &KnowledgeApp{}
}

// HandleUpload validates, deduplicates, stores, and indexes an uploaded file.
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

	qDir := filepath.Join(common.FileDir, quarantineDir)
	if !gfile.Exists(qDir) {
		if err := gfile.Mkdir(qDir); err != nil {
			return nil, fmt.Errorf("创建隔离目录失败: %s: %w", qDir, err)
		}
	}

	safeName := sanitizeFilename(input.Filename)
	sourceKey := uploadSourcePrefix + safeName
	uniqueName := fmt.Sprintf("%s_%s", uuid.New().String()[:8], safeName)
	quarantinePath := filepath.Join(qDir, uniqueName)

	if err := os.WriteFile(quarantinePath, input.Content, 0o644); err != nil {
		return nil, fmt.Errorf("保存文件失败: %w", err)
	}

	contentHash := computeSHA256(input.Content)

	if !gfile.Exists(common.FileDir) {
		if err := gfile.Mkdir(common.FileDir); err != nil {
			_ = os.Remove(quarantinePath)
			return nil, fmt.Errorf("创建目录失败: %s: %w", common.FileDir, err)
		}
	}

	existingRecords, err := listUploadRecordsBySourceKey(common.FileDir, sourceKey)
	if err != nil {
		_ = os.Remove(quarantinePath)
		return nil, fmt.Errorf("读取上传记录失败: %w", err)
	}
	if duplicate, ok := findDuplicateUploadRecord(existingRecords, contentHash); ok {
		_ = os.Remove(quarantinePath)
		status := duplicate.IndexStatus
		if status == "" {
			status = "ready"
		}
		return &UploadResult{
			FileName: safeName,
			FileSize: duplicate.FileSize,
			FileID:   duplicate.StoredFilename,
			Status:   status,
		}, nil
	}

	finalPath := filepath.Join(common.FileDir, uniqueName)
	if err := os.Rename(quarantinePath, finalPath); err != nil {
		_ = os.Remove(quarantinePath)
		return nil, fmt.Errorf("移动文件失败: %w", err)
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
		FileSize:         input.Size,
		MIMEType:         input.MIMEType,
		IndexStatus:      "indexing",
	}
	if err := writeUploadMetadata(finalPath, record); err != nil {
		_ = os.Remove(finalPath)
		return nil, fmt.Errorf("写入上传记录失败: %w", err)
	}

	go func() {
		indexCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		if err := buildIntoIndex(indexCtx, finalPath); err != nil {
			g.Log().Warningf(indexCtx, "async indexing failed for %s: %v", finalPath, err)
			record.IndexStatus = "failed"
			_ = writeUploadMetadata(finalPath, record)
			return
		}
		record.IndexStatus = "ready"
		_ = writeUploadMetadata(finalPath, record)
		if err := cleanupReplacedUploadRecords(existingRecords, record.StoredFilename); err != nil {
			g.Log().Warningf(indexCtx, "cleanup replaced upload artifacts failed: %v", err)
		}
	}()

	return &UploadResult{
		FileName: safeName,
		FileSize: input.Size,
		FileID:   uniqueName,
		Status:   "indexing",
	}, nil
}

// HandleUploadStatus queries the indexing status of a previously uploaded file.
func (k *KnowledgeApp) HandleUploadStatus(fileID string) (*UploadStatusResult, error) {
	metadataPath := uploadMetadataPath(filepath.Join(common.FileDir, fileID))
	record, err := readUploadRecord(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("文件记录不存在: %s", fileID)
	}
	status := record.IndexStatus
	if status == "" {
		status = "ready"
	}
	return &UploadStatusResult{FileID: fileID, Status: status}, nil
}

// IsAllowedExtension reports whether the given file extension (with leading dot) is allowed.
func IsAllowedExtension(ext string) bool {
	return allowedExtensions[ext]
}

// AllowedExtensions returns the set of allowed file extensions (for controller error messages).
func AllowedExtensions() []string {
	return allowedExtensionList()
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

// MaxUploadSize returns the configured maximum upload size in bytes.
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

func computeSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
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
