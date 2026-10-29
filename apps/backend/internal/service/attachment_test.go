package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type attachmentFixture struct {
	svc AttachmentService
	db  *gorm.DB
}

func newAttachmentFixture(t *testing.T) attachmentFixture {
	t.Helper()
	db := openTestDB(t, &model.Attachment{}, &model.AttachmentContent{}, &model.Partner{})
	return attachmentFixture{
		db:  db,
		svc: NewAttachmentService(repository.NewAttachmentRepository(db)),
	}
}

func (f attachmentFixture) addPartner(t *testing.T, code string) uint {
	t.Helper()
	p := &model.Partner{
		Code: code, Name: code, Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	if err := f.db.Create(p).Error; err != nil {
		t.Fatalf("建主体失败: %v", err)
	}
	return p.ID
}

// 最小可识别样本：http.DetectContentType 只看魔数，不需要完整文件。
var (
	pdfBytes  = []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n%%EOF\n")
	pngBytes  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	htmlBytes = []byte("<html><script>alert(1)</script></html>")
)

// 扩展名与内容必须对得上：只看扩展名，改个名字就能把可执行文件塞进来；
// 只看内容，一个 .pdf 里放张 jpg 会让客户打不开。
func TestAttachmentUploadRejectsMismatch(t *testing.T) {
	f := newAttachmentFixture(t)
	ctx := context.Background()
	partnerID := f.addPartner(t, "CUS001")
	in := dto.AttachmentInput{EntityType: model.AttachPartner, EntityID: partnerID}

	t.Run("扩展名不在白名单", func(t *testing.T) {
		_, err := f.svc.Upload(ctx, in, "payload.exe", pdfBytes)
		wantCode(t, err, errs.CodeInvalidParam, "可执行文件")
	})

	t.Run("扩展名与内容不符", func(t *testing.T) {
		// 一个 .pdf 里放 PNG：客户拿到打不开，而系统里看着是"PDF"。
		_, err := f.svc.Upload(ctx, in, "scan.pdf", pngBytes)
		wantCode(t, err, errs.CodeInvalidParam, "内容不是 pdf")
	})

	t.Run("HTML 改名成 pdf 也不行", func(t *testing.T) {
		_, err := f.svc.Upload(ctx, in, "notice.pdf", htmlBytes)
		wantCode(t, err, errs.CodeInvalidParam, "伪装成 pdf 的 html")
	})

	t.Run("空文件", func(t *testing.T) {
		_, err := f.svc.Upload(ctx, in, "empty.pdf", nil)
		wantCode(t, err, errs.CodeInvalidParam, "空文件")
	})

	t.Run("超过上限", func(t *testing.T) {
		huge := append(append([]byte{}, pdfBytes...), make([]byte, MaxAttachmentSize)...)
		_, err := f.svc.Upload(ctx, in, "big.pdf", huge)
		wantCode(t, err, errs.CodeInvalidParam, "超过 10MB")
	})
}

// 文件名里的路径与控制字符要摘掉：它们会破坏响应头，也能用于越权读取。
func TestAttachmentFileNameCleaning(t *testing.T) {
	f := newAttachmentFixture(t)
	ctx := context.Background()
	partnerID := f.addPartner(t, "CUS001")
	in := dto.AttachmentInput{EntityType: model.AttachPartner, EntityID: partnerID}

	m, err := f.svc.Upload(ctx, in, "../../etc/pass\"wd.pdf", pdfBytes)
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	if strings.ContainsAny(m.FileName, `/\`) || strings.Contains(m.FileName, "\"") {
		t.Errorf("文件名应剥掉路径与引号，得到 %q", m.FileName)
	}
	if m.FileName != "passwd.pdf" {
		t.Errorf("清理后的文件名应为 passwd.pdf，得到 %q", m.FileName)
	}

	if _, err := f.svc.Upload(ctx, in, "..", pdfBytes); err == nil {
		t.Error("文件名只有 .. 时应当拒绝")
	}
}

func TestAttachmentUploadStoresMetadata(t *testing.T) {
	f := newAttachmentFixture(t)
	ctx := WithActor(context.Background(), scopeActor(42, model.RoleStaff))
	partnerID := f.addPartner(t, "CUS001")

	m, err := f.svc.Upload(ctx,
		dto.AttachmentInput{EntityType: model.AttachPartner, EntityID: partnerID, Remark: "营业执照"},
		"license.pdf", pdfBytes)
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}

	if m.MimeType != "application/pdf" {
		t.Errorf("MimeType 应记录嗅探结果，得到 %q", m.MimeType)
	}
	if m.Size != int64(len(pdfBytes)) {
		t.Errorf("大小应为 %d，得到 %d", len(pdfBytes), m.Size)
	}
	// 摘要用于事后核对"这份文件是不是当初那一份"，算错等于没有。
	want := sha256.Sum256(pdfBytes)
	if m.SHA256 != hex.EncodeToString(want[:]) {
		t.Errorf("SHA256 不对: %q", m.SHA256)
	}
	if m.UploadedBy != 42 {
		t.Errorf("上传人应记成 42，得到 %d", m.UploadedBy)
	}

	// 下载要能拿回原字节。
	_, got, err := f.svc.Download(ctx, m.ID)
	if err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	if !bytes.Equal(got, pdfBytes) {
		t.Error("下载内容与上传内容不一致")
	}
}

// 挂到不存在的实体上比拒绝更难查，所以这里要求 404。
func TestAttachmentRequiresExistingTarget(t *testing.T) {
	f := newAttachmentFixture(t)
	ctx := context.Background()

	_, err := f.svc.Upload(ctx,
		dto.AttachmentInput{EntityType: model.AttachPartner, EntityID: 9999},
		"a.pdf", pdfBytes)
	wantCode(t, err, errs.CodeNotFound, "目标不存在")

	// 未知的实体类型同样拒绝，而不是建出一条谁也查不到的附件。
	_, err = f.svc.Upload(ctx,
		dto.AttachmentInput{EntityType: "spaceship", EntityID: 1}, "a.pdf", pdfBytes)
	wantCode(t, err, errs.CodeNotFound, "未知实体类型")
}

func TestAttachmentDeleteKeepsContent(t *testing.T) {
	f := newAttachmentFixture(t)
	ctx := context.Background()
	partnerID := f.addPartner(t, "CUS001")

	m, err := f.svc.Upload(ctx,
		dto.AttachmentInput{EntityType: model.AttachPartner, EntityID: partnerID},
		"a.pdf", pdfBytes)
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}

	if err := f.svc.Delete(ctx, m.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	// 软删：元数据查不到，但内容仍留着——删错了还能从库里捞回来。
	if _, _, err := f.svc.Download(ctx, m.ID); err == nil {
		t.Error("删掉之后不该还能下载")
	}
	var n int64
	if err := f.db.Model(&model.AttachmentContent{}).Count(&n).Error; err != nil {
		t.Fatalf("统计内容失败: %v", err)
	}
	if n != 1 {
		t.Errorf("内容应保留（软删），得到 %d 行", n)
	}
}
