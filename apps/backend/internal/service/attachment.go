package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// MaxAttachmentSize 是单个附件的上限，10MB。
//
// 提单、报关单、验货报告的扫描件都在这个量级以内；而内容存库，
// 放到 100MB 只会让备份、导出与列表页的可控性一起变差。
// handler 也用这个值做读取上限，两处必须一致，所以导出。
const MaxAttachmentSize int64 = 10 << 20

// allowedAttachmentTypes 是唯一允许上传的类型：扩展名 → 期望的嗅探结果。
//
// 白名单而不是黑名单：允许「什么都收」再过滤，等于把安全性押在未来的自己身上。
// 只收 PDF 与三种位图——单据与扫描件就这些。docx / xlsx 是 zip 容器，
// 嗅探分不出「一个真正的 Word」和「一个改了扩展名的压缩炸弹」，
// 真需要时再单独设计，不能顺手放开。
var allowedAttachmentTypes = map[string]string{
	".pdf":  "application/pdf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

// AttachmentService 管挂在主体与单据上的文件。
//
// 它不关心挂的是什么：类型与目标的合法性由 repository 的那张清单判定，
// 业务单据不需要为「我能不能有附件」写代码。
type AttachmentService interface {
	// Upload 存一份文件。文件名与内容都单独传，不塞进 DTO：
	// 它们来自 multipart 的 part，而不是客户端可以随意声明的 JSON 字段。
	Upload(ctx context.Context, in dto.AttachmentInput, fileName string, content []byte) (*model.Attachment, error)
	List(ctx context.Context, q dto.AttachmentQuery) ([]model.Attachment, error)
	// Download 返回元数据与内容。handler 要把它们写进响应流，包成 JSON 就没法下载了。
	Download(ctx context.Context, id uint) (*model.Attachment, []byte, error)
	Delete(ctx context.Context, id uint) error
}

type attachmentService struct {
	repo repository.AttachmentRepository
	log  *slog.Logger
}

type AttachmentOption func(*attachmentService)

func WithAttachmentLogger(l *slog.Logger) AttachmentOption {
	return func(s *attachmentService) { s.log = l }
}

func NewAttachmentService(
	repo repository.AttachmentRepository, opts ...AttachmentOption,
) AttachmentService {
	s := &attachmentService{repo: repo, log: slog.Default()}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *attachmentService) Upload(
	ctx context.Context, in dto.AttachmentInput, fileName string, content []byte,
) (*model.Attachment, error) {
	name, err := cleanFileName(fileName)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, errs.BadRequest("文件是空的")
	}
	if int64(len(content)) > MaxAttachmentSize {
		return nil, errs.BadRequest(
			fmt.Sprintf("附件不能超过 %d MB", MaxAttachmentSize>>20))
	}

	ext := strings.ToLower(filepath.Ext(name))
	expected, ok := allowedAttachmentTypes[ext]
	if !ok {
		return nil, errs.BadRequest("只支持 PDF、JPG、PNG、WebP，收到的是 " + orUnknownExt(ext))
	}
	// 扩展名与内容必须对得上：只看扩展名，改个名字就能把可执行文件塞进来；
	// 只看内容，一个 .pdf 里放张 jpg 会让客户打不开——两种都要拦。
	if detected := http.DetectContentType(content); detected != expected {
		return nil, errs.BadRequest(fmt.Sprintf(
			"%s 的内容不是 %s（识别为 %s）", name, expected, detected))
	}

	exists, err := s.repo.EntityExists(ctx, in.EntityType, in.EntityID)
	if err != nil {
		return nil, err
	}
	if !exists {
		// 未知的 entity_type 也走这里：挂到不存在的东西上，比拒绝更难查。
		return nil, errs.ErrNotFound.WithMsg("附件要挂的记录不存在")
	}

	sum := sha256.Sum256(content)
	m := &model.Attachment{
		EntityType: in.EntityType,
		EntityID:   in.EntityID,
		FileName:   name,
		MimeType:   expected,
		Size:       int64(len(content)),
		SHA256:     hex.EncodeToString(sum[:]),
		Remark:     strings.TrimSpace(in.Remark),
	}
	if actor, ok := ActorFrom(ctx); ok {
		m.UploadedBy = actor.UserID
	}

	if err := s.repo.Create(ctx, m, content); err != nil {
		return nil, err
	}
	s.log.Info("上传附件", "id", m.ID, "entity", m.EntityType,
		"entity_id", m.EntityID, "file", m.FileName, "bytes", m.Size)
	return m, nil
}

func (s *attachmentService) List(
	ctx context.Context, q dto.AttachmentQuery,
) ([]model.Attachment, error) {
	return s.repo.List(ctx, repository.AttachmentFilter{
		EntityType: q.EntityType,
		EntityID:   q.EntityID,
	})
}

func (s *attachmentService) Download(
	ctx context.Context, id uint,
) (*model.Attachment, []byte, error) {
	m, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	content, err := s.repo.FindContent(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return m, content, nil
}

func (s *attachmentService) Delete(ctx context.Context, id uint) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.log.Info("删除附件", "id", id)
	return nil
}

// cleanFileName 只留文件名本身。
//
// 浏览器可能带上路径（老 IE 会发完整本地路径），而这个名字要落库、
// 还要进 Content-Disposition 响应头——带路径或引号会让下载端存成一个奇怪的目录。
func cleanFileName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	// 控制字符与引号会破坏响应头格式，直接摘掉而不是拒绝：
	// 用户没法从「文件名不合法」里知道该改哪一步。
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		return "", errs.BadRequest("文件名不合法")
	}
	if len([]rune(name)) > 120 {
		return "", errs.BadRequest("文件名太长，上限 120 字")
	}
	return name, nil
}

func orUnknownExt(ext string) string {
	if ext == "" {
		return "没有扩展名的文件"
	}
	return ext
}
