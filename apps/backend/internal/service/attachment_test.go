package service

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

type attachmentFixture struct {
	svc       AttachmentService
	partnerID uint
}

func newAttachmentFixture(t *testing.T) attachmentFixture {
	t.Helper()
	ctx := context.Background()
	db := openTestDB(t,
		&model.Partner{}, &model.Attachment{}, &model.AttachmentContent{})

	partnerRepo := repository.NewPartnerRepository(db)
	p := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	if err := partnerRepo.Create(ctx, p); err != nil {
		t.Fatalf("建主体失败: %v", err)
	}

	return attachmentFixture{
		svc:       NewAttachmentService(repository.NewAttachmentRepository(db)),
		partnerID: p.ID,
	}
}

// pdfBytes 造一段最小 PDF：只要开头是 %PDF- 就够 http.DetectContentType 认出来，
// 这里不追求文件能打开，测的是"收不收、存不存得住、下不下来"。
func pdfBytes(body string) []byte {
	return []byte("%PDF-1.4\n" + body + "\n%%EOF\n")
}

func (f attachmentFixture) partnerInput() dto.AttachmentInput {
	return dto.AttachmentInput{EntityType: model.AttachPartner, EntityID: f.partnerID}
}

func TestAttachmentUploadListDownloadDelete(t *testing.T) {
	ctx := context.Background()
	f := newAttachmentFixture(t)
	content := pdfBytes("1 0 obj << /Type /Catalog >> endobj")

	m, err := f.svc.Upload(ctx, f.partnerInput(), "提单.pdf", content)
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	if m.MimeType != "application/pdf" {
		t.Errorf("类型 = %q，期望 application/pdf", m.MimeType)
	}
	if m.Size != int64(len(content)) {
		t.Errorf("大小 = %d，期望 %d", m.Size, len(content))
	}
	// sha256 的十六进制是 64 个字符：存错了长度就说明算的不是整份文件。
	if len(m.SHA256) != 64 {
		t.Errorf("sha256 = %q，期望 64 位十六进制", m.SHA256)
	}

	list, err := f.svc.List(ctx, dto.AttachmentQuery{
		EntityType: model.AttachPartner, EntityID: f.partnerID,
	})
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if len(list) != 1 || list[0].ID != m.ID {
		t.Fatalf("列表 = %+v，期望只有刚上传的那一条", list)
	}

	meta, got, err := f.svc.Download(ctx, m.ID)
	if err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	if meta.FileName != "提单.pdf" {
		t.Errorf("文件名 = %q，期望「提单.pdf」", meta.FileName)
	}
	if !bytes.Equal(got, content) {
		t.Error("下载到的内容与上传的不一致")
	}

	if err := f.svc.Delete(ctx, m.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	list, err = f.svc.List(ctx, dto.AttachmentQuery{
		EntityType: model.AttachPartner, EntityID: f.partnerID,
	})
	if err != nil {
		t.Fatalf("删除后列表失败: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("软删后仍列出 %d 条", len(list))
	}
	// 软删：内容留着，追溯时要能捞回原文件。
	if _, _, err := f.svc.Download(ctx, m.ID); err == nil {
		t.Error("已删除的附件不该还能下载")
	}
}

// 文件名只留最后一段：浏览器（尤其老 IE）会发完整本地路径，
// 而这个名字要进 Content-Disposition，带路径会把客户的文件存到奇怪的地方。
func TestAttachmentFileNameCleaning(t *testing.T) {
	ctx := context.Background()
	f := newAttachmentFixture(t)

	m, err := f.svc.Upload(ctx, f.partnerInput(), `C:\Users\me\Desktop\报关单.pdf`, pdfBytes("x"))
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	if m.FileName != "报关单.pdf" {
		t.Errorf("文件名 = %q，期望去掉路径后的「报关单.pdf」", m.FileName)
	}
}

// 白名单里的四种都要能通过嗅探：Go 的 DetectContentType 对各格式的支持是分版本加的，
// 少一个分支的表现是「这类文件一律被拒」，而用户看不出为什么。
func TestAttachmentAllowedTypes(t *testing.T) {
	ctx := context.Background()
	f := newAttachmentFixture(t)

	cases := []struct {
		file    string
		content []byte
		mime    string
	}{
		{"单据.pdf", pdfBytes("x"), "application/pdf"},
		{"照片.jpg", []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01"), "image/jpeg"},
		{"照片.jpeg", []byte("\xff\xd8\xff\xe1\x00\x10JFIF\x00\x01"), "image/jpeg"},
		{"照片.png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), "image/png"},
		{"照片.webp", []byte("RIFF\x24\x00\x00\x00WEBPVP8 "), "image/webp"},
	}

	for _, tc := range cases {
		t.Run(tc.mime, func(t *testing.T) {
			m, err := f.svc.Upload(ctx, f.partnerInput(), tc.file, tc.content)
			if err != nil {
				t.Fatalf("上传 %s 失败: %v", tc.file, err)
			}
			if m.MimeType != tc.mime {
				t.Errorf("类型 = %q，期望 %q", m.MimeType, tc.mime)
			}
		})
	}
}

func TestAttachmentUploadRejects(t *testing.T) {
	ctx := context.Background()
	f := newAttachmentFixture(t)

	cases := []struct {
		name    string
		input   dto.AttachmentInput
		file    string
		content []byte
	}{
		{
			name: "空文件", input: f.partnerInput(), file: "空.pdf", content: nil,
		},
		{
			name: "扩展名不在白名单", input: f.partnerInput(), file: "报价.xlsx",
			content: []byte("PK\x03\x04"),
		},
		{
			// 改个后缀就能把别的东西塞进来，是最常见的一类：内容与扩展名必须对得上。
			name: "扩展名与内容不符", input: f.partnerInput(), file: "其实是图.pdf",
			content: []byte("\x89PNG\r\n\x1a\n0000"),
		},
		{
			name: "没有扩展名", input: f.partnerInput(), file: "扫描件",
			content: pdfBytes("x"),
		},
		{
			name: "超过大小上限", input: f.partnerInput(), file: "超大.pdf",
			content: append(pdfBytes(strings.Repeat("x", int(MaxAttachmentSize))), 'x'),
		},
		{
			name: "目标记录不存在", file: "提单.pdf",
			input:   dto.AttachmentInput{EntityType: model.AttachPartner, EntityID: 9999},
			content: pdfBytes("x"),
		},
		{
			name: "未知的挂载类型", file: "提单.pdf",
			input:   dto.AttachmentInput{EntityType: "spaceship", EntityID: 1},
			content: pdfBytes("x"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.svc.Upload(ctx, tc.input, tc.file, tc.content); err == nil {
				t.Fatal("应该被拒绝，但上传成功了")
			}
		})
	}
}
