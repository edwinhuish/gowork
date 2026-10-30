package handler

import (
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/server/middleware"
	"github.com/edwinhuish/gowork/internal/service"
)

// ProductImageHandler 处理产品图片的上传、排序、替换、删除与取图。
//
// 上传与取图不走 JSON：前者是 multipart，后者是二进制流。失败时仍走统一响应体。
type ProductImageHandler struct {
	images service.ProductImageService
}

func NewProductImageHandler(images service.ProductImageService) *ProductImageHandler {
	return &ProductImageHandler{images: images}
}

// List GET /api/v1/products/:id/images
func (h *ProductImageHandler) List(c *gin.Context) {
	productID, ok := parseID(c)
	if !ok {
		return
	}
	list, err := h.images.List(c.Request.Context(), productID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}

// Upload POST /api/v1/products/:id/images
//
// 一次可提交多张（上限见 service.MaxProductImagesPerUpload）：
// 逐张发请求时，用户选了六张就要等六次往返，中途失败还分不清进度。
func (h *ProductImageHandler) Upload(c *gin.Context) {
	productID, ok := parseID(c)
	if !ok {
		return
	}

	files, ok := readMultipartFiles(c, "files", service.MaxProductImagesPerUpload)
	if !ok {
		return
	}
	list, err := h.images.Upload(c.Request.Context(), productID, files)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, list)
}

// Replace PUT /api/v1/products/:id/images/:imageId
func (h *ProductImageHandler) Replace(c *gin.Context) {
	productID, ok := parseID(c)
	if !ok {
		return
	}
	imageID, ok := parseUintParam(c, "imageId", "图片")
	if !ok {
		return
	}

	header, err := c.FormFile("file")
	if err != nil {
		if middleware.HandleBodyTooLarge(c, err) {
			return
		}
		response.Fail(c, errs.BadRequest("替换需要 multipart/form-data 与 file 字段"))
		return
	}
	content, err := readUploadedFile(header)
	if err != nil {
		response.Fail(c, err)
		return
	}

	m, err := h.images.Replace(c.Request.Context(), productID, imageID, header.Filename, content)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/products/:id/images/:imageId
func (h *ProductImageHandler) Delete(c *gin.Context) {
	productID, ok := parseID(c)
	if !ok {
		return
	}
	imageID, ok := parseUintParam(c, "imageId", "图片")
	if !ok {
		return
	}
	if err := h.images.Delete(c.Request.Context(), productID, imageID); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Reorder PUT /api/v1/products/:id/images/order
//
// 提交完整排列而不是「上移一格」：后者要求客户端与库里的位置严格对齐，
// 两个人同时调整就会错位；全量提交让服务端能一次判定集合是否一致。
func (h *ProductImageHandler) Reorder(c *gin.Context) {
	productID, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ProductImageOrderInput
	if !bindJSON(c, &in) {
		return
	}
	list, err := h.images.Reorder(c.Request.Context(), productID, in.IDs)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}

// Content GET /api/v1/products/:id/images/:imageId/content?size=thumb
//
// 图片走 Authorization 头而不是公开地址：本系统的鉴权只有一套，
// 另开一个带签名的公开 URL 等于多一条会进日志、会被转发的取图通道。
// 代价是 <img src> 用不了，前端要带令牌取 blob。
func (h *ProductImageHandler) Content(c *gin.Context) {
	productID, ok := parseID(c)
	if !ok {
		return
	}
	imageID, ok := parseUintParam(c, "imageId", "图片")
	if !ok {
		return
	}
	thumb := c.Query("size") == "thumb"

	img, data, err := h.images.Content(c.Request.Context(), productID, imageID, thumb)
	if err != nil {
		response.Fail(c, err)
		return
	}

	// 缩略图统一是 JPEG（生成时就转好了），原图按嗅探结果。
	contentType := img.MimeType
	etag := `"` + img.SHA256 + `"`
	if thumb {
		contentType = "image/jpeg"
		etag = `"` + img.SHA256 + `-thumb"`
	}

	// 替换会换 sha256，ETag 随之变化；浏览器再次进入列表时走 304 而不是重传。
	c.Header("ETag", etag)
	c.Header("Cache-Control", "private, max-age=86400")
	c.Header("X-Content-Type-Options", "nosniff")
	if match := c.GetHeader("If-None-Match"); match != "" && strings.Contains(match, etag) {
		c.Status(http.StatusNotModified)
		return
	}

	c.Header("Content-Disposition", "inline")
	c.Data(http.StatusOK, contentType, data)
}

// readMultipartFiles 读取 multipart 里指定字段的全部文件。
//
// limit 在解析之前就卡住：让用户传 12 张 5MB 再回一句「最多 4 张」，
// 白等的时间比错误提示本身更难忍。
func readMultipartFiles(c *gin.Context, field string, limit int) ([]dto.ProductImageFile, bool) {
	form, err := c.MultipartForm()
	if err != nil {
		if middleware.HandleBodyTooLarge(c, err) {
			return nil, false
		}
		response.Fail(c, errs.BadRequest("上传需要 multipart/form-data 与 "+field+" 字段"))
		return nil, false
	}
	headers := form.File[field]
	if len(headers) == 0 {
		response.Fail(c, errs.BadRequest("请选择要上传的图片"))
		return nil, false
	}
	if len(headers) > limit {
		response.Fail(c, errs.BadRequest(
			"单次最多上传 "+strconv.Itoa(limit)+" 张，请分批提交"))
		return nil, false
	}

	files := make([]dto.ProductImageFile, 0, len(headers))
	for _, header := range headers {
		content, err := readUploadedFile(header)
		if err != nil {
			response.Fail(c, err)
			return nil, false
		}
		files = append(files, dto.ProductImageFile{FileName: header.Filename, Content: content})
	}
	return files, true
}

// readUploadedFile 读出单个上传文件，多读 1 字节用于判超限。
func readUploadedFile(header *multipart.FileHeader) ([]byte, error) {
	f, err := header.Open()
	if err != nil {
		return nil, errs.BadRequest("读取上传文件失败")
	}
	defer func() { _ = f.Close() }()

	// 多读 1 字节：刚够判断「超限」，不为超限的文件把整个文件读进内存。
	content, err := io.ReadAll(io.LimitReader(f, service.MaxProductImageSize+1))
	if err != nil {
		return nil, errs.BadRequest("读取上传文件失败")
	}
	return content, nil
}
