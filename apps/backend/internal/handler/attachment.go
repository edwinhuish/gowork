package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// AttachmentHandler 处理附件上传、列表、下载与删除。
//
// 上传走 multipart/form-data，下载直接吐字节流——两者都不套 {code,message,data}：
// 文件不是 JSON，包一层反而没法选中存成文件。失败时仍走统一响应体。
type AttachmentHandler struct {
	attachments service.AttachmentService
}

func NewAttachmentHandler(a service.AttachmentService) *AttachmentHandler {
	return &AttachmentHandler{attachments: a}
}

// List GET /api/v1/attachments?entity_type=&entity_id=
func (h *AttachmentHandler) List(c *gin.Context) {
	var q dto.AttachmentQuery
	if !bindQuery(c, &q) {
		return
	}
	list, err := h.attachments.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}

// Upload POST /api/v1/attachments
//
// 文件与「挂在哪」一次提交，不走「先建记录、再 PUT 内容」：
// 两步之间断掉就会留下一条没有文件的附件记录，而它看上去完全正常。
func (h *AttachmentHandler) Upload(c *gin.Context) {
	var in dto.AttachmentInput
	if err := c.ShouldBind(&in); err != nil {
		response.Fail(c, errs.BadRequest("上传需要 entity_type、entity_id 与 file"))
		return
	}

	header, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, errs.BadRequest("上传需要 entity_type、entity_id 与 file"))
		return
	}
	f, err := header.Open()
	if err != nil {
		response.Fail(c, fmt.Errorf("读取上传文件: %w", err))
		return
	}
	defer func() { _ = f.Close() }()

	// 多读 1 字节：刚够判断「超限」，不为超限的文件把整个文件读进内存。
	content, err := io.ReadAll(io.LimitReader(f, service.MaxAttachmentSize+1))
	if err != nil {
		response.Fail(c, fmt.Errorf("读取上传文件: %w", err))
		return
	}

	m, err := h.attachments.Upload(c.Request.Context(), in, header.Filename, content)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Download GET /api/v1/attachments/:id/download
func (h *AttachmentHandler) Download(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, content, err := h.attachments.Download(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", attachmentDisposition(m.FileName))
	c.Data(http.StatusOK, m.MimeType, content)
}

// Delete DELETE /api/v1/attachments/:id
func (h *AttachmentHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.attachments.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// attachmentDisposition 生成下载头。
//
// 文件名可能是中文：直接塞进 filename="..." 在不同浏览器上会被截断或乱码。
// 所以两个都给——filename 是 ASCII 兜底（老客户端），filename* 是 RFC 5987 的
// UTF-8 形式（现代浏览器据此存成原始中文名）。
func attachmentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r > 0x7e || r < 0x20 {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`attachment; filename=%q; filename*=UTF-8''%s`,
		ascii, url.PathEscape(name))
}
