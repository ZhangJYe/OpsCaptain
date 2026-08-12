package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/app"
	"context"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) FileUpload(ctx context.Context, req *v1.FileUploadReq) (res *v1.FileUploadRes, err error) {
	r := g.RequestFromCtx(ctx)
	uploadFile := r.GetUploadFile("file")
	if uploadFile == nil {
		return nil, gerror.New("请上传文件")
	}

	ext := strings.ToLower(filepath.Ext(uploadFile.Filename))
	if !app.IsAllowedExtension(ext) {
		return nil, gerror.Newf("不支持的文件类型: %s, 允许: %v", ext, app.AllowedExtensions())
	}

	if maxSize := app.MaxUploadSize(ctx); uploadFile.Size > maxSize {
		return nil, gerror.Newf("文件大小 %d 字节超过限制 %d 字节", uploadFile.Size, maxSize)
	}

	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = uploadFile.FileHeader.Header.Get("Content-Type")
	}

	f, err := uploadFile.Open()
	if err != nil {
		return nil, gerror.Wrapf(err, "打开上传文件失败")
	}
	content, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, gerror.Wrapf(err, "读取文件内容失败")
	}

	result, err := c.knowledgeApp.HandleUpload(ctx, &app.UploadInput{
		Filename: uploadFile.Filename,
		MIMEType: mimeType,
		Size:     uploadFile.Size,
		Content:  content,
	})
	if err != nil {
		return nil, gerror.New(err.Error())
	}

	return &v1.FileUploadRes{
		FileName: result.FileName,
		FileSize: result.FileSize,
		FileID:   result.FileID,
		Status:   result.Status,
	}, nil
}

func (c *ControllerV1) UploadStatus(ctx context.Context, req *v1.UploadStatusReq) (res *v1.UploadStatusRes, err error) {
	result, err := c.knowledgeApp.HandleUploadStatus(ctx, req.FileID)
	if err != nil {
		return nil, gerror.New(err.Error())
	}
	return &v1.UploadStatusRes{
		FileID: result.FileID,
		Status: result.Status,
	}, nil
}

func (c *ControllerV1) KnowledgeDocumentList(ctx context.Context, req *v1.KnowledgeDocumentListReq) (res *v1.KnowledgeDocumentListRes, err error) {
	items, err := c.knowledgeApp.ListDocuments(ctx)
	if err != nil {
		return nil, gerror.New(err.Error())
	}
	result := make([]v1.KnowledgeDocumentItem, 0, len(items))
	for _, item := range items {
		result = append(result, knowledgeDocumentResponse(item))
	}
	return &v1.KnowledgeDocumentListRes{Items: result}, nil
}

func (c *ControllerV1) KnowledgeDocumentDelete(ctx context.Context, req *v1.KnowledgeDocumentDeleteReq) (res *v1.KnowledgeDocumentDeleteRes, err error) {
	if err := c.knowledgeApp.DeleteDocument(ctx, req.FileID); err != nil {
		return nil, gerror.New(err.Error())
	}
	return &v1.KnowledgeDocumentDeleteRes{}, nil
}

func (c *ControllerV1) KnowledgeDocumentReindex(ctx context.Context, req *v1.KnowledgeDocumentReindexReq) (res *v1.KnowledgeDocumentReindexRes, err error) {
	item, err := c.knowledgeApp.RetryDocumentIndex(ctx, req.FileID)
	if err != nil {
		return nil, gerror.New(err.Error())
	}
	return &v1.KnowledgeDocumentReindexRes{Item: knowledgeDocumentResponse(*item)}, nil
}

func knowledgeDocumentResponse(item app.KnowledgeDocument) v1.KnowledgeDocumentItem {
	return v1.KnowledgeDocumentItem{
		FileID: item.FileID, FileName: item.FileName, FileSize: item.FileSize,
		MIMEType: item.MIMEType, Status: item.Status, UploadedAt: item.UploadedAt, Version: item.Version,
	}
}
