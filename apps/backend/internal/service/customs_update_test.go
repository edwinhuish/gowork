package service

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// 报关记录描述的是"这一票货的出口"，改挂到另一票等于两票的单证都对不上。
//
// 另起一个夹具而不是复用 `customs_declaration_test.go` 里的那个：
// 那边造的是完整出货链路（含费用与状态流转），这里只需要两票货与一条记录。
type customsUpdateFixture struct {
	svc CustomsDeclarationService
	db  *gorm.DB
}

func newCustomsUpdateFixture(t *testing.T) customsUpdateFixture {
	t.Helper()
	db := openTestDB(t, &model.Shipment{}, &model.CustomsDeclaration{})
	return customsUpdateFixture{
		db: db,
		svc: NewCustomsDeclarationService(
			repository.NewCustomsDeclarationRepository(db), repository.NewShipmentRepository(db)),
	}
}

func (f customsUpdateFixture) addShipment(t *testing.T, docNo string) uint {
	t.Helper()
	s := &model.Shipment{DocNo: docNo, Status: model.DocumentStatusConfirmed}
	if err := f.db.Create(s).Error; err != nil {
		t.Fatalf("建出货单失败: %v", err)
	}
	return s.ID
}

func (f customsUpdateFixture) addDeclaration(t *testing.T, shipmentID uint) *model.CustomsDeclaration {
	t.Helper()
	m := &model.CustomsDeclaration{
		ShipmentID: shipmentID, Status: model.CustomsStatusDraft, SupervisionMode: "0110",
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建报关记录失败: %v", err)
	}
	return m
}

func TestCustomsUpdateCannotMoveShipment(t *testing.T) {
	f := newCustomsUpdateFixture(t)
	ctx := context.Background()
	a := f.addShipment(t, "SH-A")
	b := f.addShipment(t, "SH-B")
	decl := f.addDeclaration(t, a)

	_, err := f.svc.Update(ctx, decl.ID, dto.CustomsDeclarationInput{
		ShipmentID: b, Status: int8(model.CustomsStatusDraft),
	})
	wantCode(t, err, errs.CodeInvalidParam, "改挂另一票")
}

func TestCustomsUpdateValidatesAndTrims(t *testing.T) {
	f := newCustomsUpdateFixture(t)
	ctx := context.Background()
	decl := f.addDeclaration(t, f.addShipment(t, "SH-A"))

	t.Run("状态必须合法", func(t *testing.T) {
		_, err := f.svc.Update(ctx, decl.ID, dto.CustomsDeclarationInput{Status: 77})
		wantCode(t, err, errs.CodeInvalidParam, "未知状态")
	})

	t.Run("申报日期格式", func(t *testing.T) {
		_, err := f.svc.Update(ctx, decl.ID, dto.CustomsDeclarationInput{
			Status: int8(model.CustomsStatusDraft), DeclDate: "2026/10/20",
		})
		wantCode(t, err, errs.CodeInvalidParam, "日期格式")
	})

	t.Run("正常补录单号与日期", func(t *testing.T) {
		got, err := f.svc.Update(ctx, decl.ID, dto.CustomsDeclarationInput{
			Status: int8(model.CustomsStatusDeclared), DeclNo: " 531620260012345678 ",
			DeclDate: "2026-10-20", SupervisionMode: " 0110 ",
		})
		if err != nil {
			t.Fatalf("更新失败: %v", err)
		}
		// 前后空格要摘掉：单号是从 PDF 里抄的，粘进来常带空格。
		if got.DeclNo != "531620260012345678" {
			t.Errorf("单号应去掉首尾空格，得到 %q", got.DeclNo)
		}
		if got.SupervisionMode != "0110" {
			t.Errorf("监管方式应去掉首尾空格，得到 %q", got.SupervisionMode)
		}
		if got.Status != model.CustomsStatusDeclared {
			t.Errorf("状态应为已申报，得到 %v", got.Status)
		}
		if got.DeclDate == nil ||
			model.DateOnly(*got.DeclDate).Format("2006-01-02") != "2026-10-20" {
			t.Errorf("申报日期不对: %+v", got.DeclDate)
		}
	})
}
