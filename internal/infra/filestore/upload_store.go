package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const quarantineDir = "quarantine"

var safeFilenameRe = regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)

type UploadStore interface {
	SaveUpload(context.Context, UploadSaveInput) (*UploadSaveResult, error)
	UploadStatus(context.Context, string) (string, error)
	MarkUploadStatus(context.Context, string, string) error
	CleanupReplacedUploads(context.Context, string, string, string) error
	ListUploads(context.Context, string, string) ([]UploadRecord, error)
	GetUpload(context.Context, string) (*UploadRecord, error)
	DeleteUpload(context.Context, string) error
}

type LocalUploadStore struct {
	dir string
}

type UploadSaveInput struct {
	Filename     string
	MIMEType     string
	Size         int64
	Content      []byte
	SourceKind   string
	SourcePrefix string
}

type UploadSaveResult struct {
	FileName  string
	FileID    string
	FilePath  string
	FileSize  int64
	SourceKey string
	Status    string
	Duplicate bool
}

type UploadRecord struct {
	FileID      string
	FileName    string
	FilePath    string
	FileSize    int64
	MIMEType    string
	SourceKind  string
	SourceKey   string
	UploadedAt  string
	Version     int
	IndexStatus string
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

func NewLocalUploadStore(dir string) *LocalUploadStore {
	return &LocalUploadStore{dir: dir}
}

func (s *LocalUploadStore) SaveUpload(_ context.Context, input UploadSaveInput) (*UploadSaveResult, error) {
	if strings.TrimSpace(s.dir) == "" {
		return nil, fmt.Errorf("upload store dir is empty")
	}
	safeName := sanitizeFilename(input.Filename)
	sourceKey := input.SourcePrefix + safeName

	qDir := filepath.Join(s.dir, quarantineDir)
	if err := os.MkdirAll(qDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建隔离目录失败: %s: %w", qDir, err)
	}

	uniqueName := fmt.Sprintf("%s_%s", uuid.New().String()[:8], safeName)
	quarantinePath := filepath.Join(qDir, uniqueName)
	if err := os.WriteFile(quarantinePath, input.Content, 0o644); err != nil {
		return nil, fmt.Errorf("保存文件失败: %w", err)
	}

	contentHash := computeSHA256(input.Content)
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		_ = os.Remove(quarantinePath)
		return nil, fmt.Errorf("创建目录失败: %s: %w", s.dir, err)
	}

	existingRecords, err := s.listUploadRecordsBySource(input.SourceKind, sourceKey)
	if err != nil {
		_ = os.Remove(quarantinePath)
		return nil, fmt.Errorf("读取上传记录失败: %w", err)
	}
	if duplicate, ok := s.findDuplicateUploadRecord(existingRecords, contentHash); ok {
		_ = os.Remove(quarantinePath)
		status := duplicate.IndexStatus
		if status == "" {
			status = "ready"
		}
		return &UploadSaveResult{
			FileName:  safeName,
			FileSize:  duplicate.FileSize,
			FileID:    duplicate.StoredFilename,
			FilePath:  duplicate.filePath,
			SourceKey: sourceKey,
			Status:    status,
			Duplicate: true,
		}, nil
	}

	finalPath := filepath.Join(s.dir, uniqueName)
	if err := os.Rename(quarantinePath, finalPath); err != nil {
		_ = os.Remove(quarantinePath)
		return nil, fmt.Errorf("移动文件失败: %w", err)
	}

	record := uploadFileRecord{
		SourceKind:       input.SourceKind,
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

	return &UploadSaveResult{
		FileName:  safeName,
		FileSize:  input.Size,
		FileID:    uniqueName,
		FilePath:  finalPath,
		SourceKey: sourceKey,
		Status:    "indexing",
	}, nil
}

func (s *LocalUploadStore) UploadStatus(_ context.Context, fileID string) (string, error) {
	record, err := readUploadRecord(uploadMetadataPath(filepath.Join(s.dir, safeStoredFilename(fileID))))
	if err != nil {
		return "", err
	}
	if record.IndexStatus == "" {
		return "ready", nil
	}
	return record.IndexStatus, nil
}

func (s *LocalUploadStore) MarkUploadStatus(_ context.Context, fileID, status string) error {
	path := filepath.Join(s.dir, safeStoredFilename(fileID))
	record, err := readUploadRecord(uploadMetadataPath(path))
	if err != nil {
		return err
	}
	record.IndexStatus = strings.TrimSpace(status)
	return writeUploadMetadata(path, record)
}

func (s *LocalUploadStore) CleanupReplacedUploads(_ context.Context, sourceKind, sourceKey, keepStoredFilename string) error {
	records, err := s.listUploadRecordsBySource(sourceKind, sourceKey)
	if err != nil {
		return err
	}
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

func (s *LocalUploadStore) ListUploads(_ context.Context, sourceKind, sourcePrefix string) ([]UploadRecord, error) {
	records, err := s.listUploadRecordsByPrefix(sourceKind, sourcePrefix)
	if err != nil {
		return nil, err
	}
	items := make([]UploadRecord, 0, len(records))
	for _, record := range records {
		items = append(items, toUploadRecord(record))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].UploadedAt > items[j].UploadedAt
	})
	return items, nil
}

func (s *LocalUploadStore) GetUpload(_ context.Context, fileID string) (*UploadRecord, error) {
	fileID = safeStoredFilename(fileID)
	record, err := readUploadRecord(uploadMetadataPath(filepath.Join(s.dir, fileID)))
	if err != nil {
		return nil, err
	}
	record.filePath = filepath.Join(s.dir, fileID)
	record.metadataPath = uploadMetadataPath(record.filePath)
	item := toUploadRecord(record)
	return &item, nil
}

func (s *LocalUploadStore) DeleteUpload(ctx context.Context, fileID string) error {
	record, err := s.GetUpload(ctx, fileID)
	if err != nil {
		return err
	}
	return cleanupUploadRecord(uploadFileRecord{
		StoredFilename: record.FileID,
		filePath:       record.FilePath,
		metadataPath:   uploadMetadataPath(record.FilePath),
	})
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

func safeStoredFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), "..", ""))
	if name == "" || name == "." {
		return "unnamed"
	}
	return name
}

func computeSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func uploadMetadataPath(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path + ".metadata.json"
	}
	return path[:len(path)-len(ext)] + ".metadata.json"
}

func (s *LocalUploadStore) listUploadRecordsBySource(sourceKind, sourceKey string) ([]uploadFileRecord, error) {
	return s.listUploadRecords(sourceKind, func(record uploadFileRecord) bool {
		return record.SourceKey == sourceKey
	})
}

func (s *LocalUploadStore) listUploadRecordsByPrefix(sourceKind, sourcePrefix string) ([]uploadFileRecord, error) {
	return s.listUploadRecords(sourceKind, func(record uploadFileRecord) bool {
		return strings.HasPrefix(record.SourceKey, sourcePrefix)
	})
}

func (s *LocalUploadStore) listUploadRecords(sourceKind string, matches func(uploadFileRecord) bool) ([]uploadFileRecord, error) {
	entries, err := os.ReadDir(s.dir)
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
		record, err := readUploadRecord(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if record.SourceKind != sourceKind || !matches(record) {
			continue
		}
		if strings.TrimSpace(record.StoredFilename) == "" {
			continue
		}
		record.filePath = filepath.Join(s.dir, record.StoredFilename)
		record.metadataPath = filepath.Join(s.dir, entry.Name())
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

func toUploadRecord(record uploadFileRecord) UploadRecord {
	status := record.IndexStatus
	if status == "" {
		status = "ready"
	}
	return UploadRecord{
		FileID:      record.StoredFilename,
		FileName:    record.OriginalFilename,
		FilePath:    record.filePath,
		FileSize:    record.FileSize,
		MIMEType:    record.MIMEType,
		SourceKind:  record.SourceKind,
		SourceKey:   record.SourceKey,
		UploadedAt:  record.UploadedAt,
		Version:     record.Version,
		IndexStatus: status,
	}
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

func (s *LocalUploadStore) findDuplicateUploadRecord(records []uploadFileRecord, contentHash string) (uploadFileRecord, bool) {
	for _, record := range records {
		if record.ContentHash != contentHash {
			continue
		}
		if strings.TrimSpace(record.StoredFilename) == "" {
			continue
		}
		if record.filePath == "" {
			record.filePath = filepath.Join(s.dir, record.StoredFilename)
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

func cleanupUploadRecord(record uploadFileRecord) error {
	if record.filePath == "" && strings.TrimSpace(record.StoredFilename) != "" {
		record.filePath = filepath.Join(filepath.Dir(record.metadataPath), record.StoredFilename)
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
