package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

type customsFixture struct {
	declarations CustomsDeclarationService
	shipments    ShipmentService
	draftID      uint
	confirmedID  uint
}

func newCustomsFixture(t *testing.T, now time.Time) customsFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Shipment{}, &model.ShipmentAllocation{},
		&model.ShipmentCharge{}, &model.CustomsDeclaration{})

	clock := func() time.Time { return now }
	shipmentRepo := repository.NewShipmentRepository(db)
	chargeRepo := repository.NewShipmentChargeRepository(db)
	customsRepo := repository.NewCustomsDeclarationRepository(db)

	mkShipment := func(docNo string, status model.DocumentStatus) uint {
		s := &model.Shipment{DocNo: docNo, Status: status, ShipDate: now}
		if err := shipmentRepo.CreateWithAllocations(ctx, s, nil); err != nil {
			t.Fatalf("建出货批次失败: %v", err)
		}
		return s.ID
	}

	return customsFixture{
		declarations: NewCustomsDeclarationService(customsRepo, shipmentRepo),
		shipments: NewShipmentService(
			shipmentRepo, repository.NewSalesOrderRepository(db), chargeRepo, customsRepo,
			NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock)),
			repository.NewTxManager(db), stubSalesOut{t},
			WithShipmentClock(clock),
		),
		draftID:     mkShipment("SH-2610-011", model.DocumentStatusDraft),
		confirmedID: mkShipment("SH-2610-012", model.DocumentStatusConfirmed),
	}
}

// TestCustomsDeclarationMultiplePerShipment 守的是"一票多张"这个建模选择：
// 同一票货不同 HS 编码要分票申报，被退单后改单重报也是新的一条，旧的留着。
func TestCustomsDeclarationMultiplePerShipment(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newCustomsFixture(t, now)

	first, err := f.declarations.Create(ctx, dto.CustomsDeclarationInput{
		ShipmentID:      f.confirmedID,
		Status:          int8(model.CustomsStatusReleased),
		DeclNo:          "223120260001",
		DeclDate:        now.Format("2006-01-02"),
		SupervisionMode: "0110",
	})
	if err != nil {
		t.Fatalf("新增报关单失败: %v", err)
	}
	if first.DeclDate == nil {
		t.Fatal("申报日期应被解析并落库")
	}
	if first.Status.Label() != "已放行" {
		t.Errorf("状态标签应为「已放行」，得到 %q", first.Status.Label())
	}

	// 退单后改单重报：新的一条，旧的留着，否则"海关退回过什么"就查不到了。
	if _, err := f.declarations.Create(ctx, dto.CustomsDeclarationInput{
		ShipmentID: f.confirmedID,
		Status:     int8(model.CustomsStatusDraft),
	}); err != nil {
		t.Fatalf("重报的报关单应能再建一条: %v", err)
	}

	res, err := f.declarations.List(ctx, dto.CustomsDeclarationQuery{
		ShipmentID: &f.confirmedID,
	})
	if err != nil {
		t.Fatalf("查报关单失败: %v", err)
	}
	if res.Total != 2 {
		t.Errorf("同一票货应有两张报关单，得到 %d", res.Total)
	}
}

func TestCustomsDeclarationValidation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newCustomsFixture(t, now)

	cases := []struct {
		name string
		in   dto.CustomsDeclarationInput
	}{
		{
			name: "批次不存在",
			in:   dto.CustomsDeclarationInput{ShipmentID: 9999, Status: int8(model.CustomsStatusDraft)},
		},
		{
			name: "状态未定义",
			in:   dto.CustomsDeclarationInput{ShipmentID: f.draftID, Status: 50},
		},
		{
			name: "日期格式不对",
			in: dto.CustomsDeclarationInput{
				ShipmentID: f.draftID, Status: int8(model.CustomsStatusDeclared), DeclDate: "2026/10/28",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := f.declarations.Create(ctx, c.in); err == nil {
				t.Errorf("%s 时应拒绝创建", c.name)
			}
		})
	}
}

// TestShipmentLogisticsAnyStatus 守的是这条设计：提单号与船期订舱后才有，
// 那时出货单早就不在草稿了，所以物流节点不能跟着 Update 的状态限制走。
func TestShipmentLogisticsAnyStatus(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newCustomsFixture(t, now)

	etd := now.AddDate(0, 0, 3).Format("2006-01-02")
	eta := now.AddDate(0, 0, 25).Format("2006-01-02")

	got, err := f.shipments.UpdateLogistics(ctx, f.confirmedID, dto.ShipmentLogisticsInput{
		BLNo:   "COSU1234567890",
		Vessel: "COSCO SHIPPING / 028E",
		ETD:    etd,
		ETA:    eta,
	})
	if err != nil {
		t.Fatalf("已确认的出货单应能记物流节点，得到错误: %v", err)
	}
	if got.BLNo != "COSU1234567890" || got.Vessel != "COSCO SHIPPING / 028E" {
		t.Errorf("提单号与船名航次未落库：%q / %q", got.BLNo, got.Vessel)
	}
	if got.ETD == nil || got.ETA == nil {
		t.Error("ETD / ETA 未落库")
	}

	// 到港早于开船一定是录错了。
	if _, err := f.shipments.UpdateLogistics(ctx, f.confirmedID, dto.ShipmentLogisticsInput{
		ETD: eta,
		ETA: etd,
	}); err == nil {
		t.Error("ETA 早于 ETD 时应拒绝")
	}

	// 留空表示未定：会把已填的值清掉，而不是保留旧值。
	cleared, err := f.shipments.UpdateLogistics(ctx, f.confirmedID, dto.ShipmentLogisticsInput{})
	if err != nil {
		t.Fatalf("清空物流节点失败: %v", err)
	}
	if cleared.ETD != nil || cleared.ETA != nil {
		t.Error("留空应把 ETD / ETA 清成空")
	}
}

func TestShipmentDeleteBlockedByDeclarations(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newCustomsFixture(t, now)

	decl, err := f.declarations.Create(ctx, dto.CustomsDeclarationInput{
		ShipmentID: f.draftID,
		Status:     int8(model.CustomsStatusDraft),
	})
	if err != nil {
		t.Fatalf("新增报关单失败: %v", err)
	}

	if err := f.shipments.Delete(ctx, f.draftID); err == nil {
		t.Error("有报关单挂着的批次应拒绝删除")
	}

	if err := f.declarations.Delete(ctx, decl.ID); err != nil {
		t.Fatalf("删除报关单失败: %v", err)
	}
	if err := f.shipments.Delete(ctx, f.draftID); err != nil {
		t.Errorf("报关单删完后应能删批次，得到错误: %v", err)
	}
}
