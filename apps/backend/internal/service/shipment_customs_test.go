package service

import (
	"testing"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// TestSnapshotCustoms 是 T-55 后半的验收点：报关要素在出货时抄到分配行上，
// 之后不再跟随产品变化。
//
// 不快照的后果很具体：产品上改一次 HS 编码，历史出货的报关单与产地证
// 就复现不出当时的申报内容，而报关资料是要留档备查的。
func TestSnapshotCustoms(t *testing.T) {
	p := &model.Product{
		HSCode: "8302", DeclaredNameCN: "不锈钢合页", DeclaredNameEN: "Hinge",
		Material: "不锈钢，厨房用", OriginCountry: "China", LegalUnit: "千克",
	}
	a := &model.ShipmentAllocation{}
	snapshotCustoms(a, p)
	if a.HSCode != "8302" || a.OriginCountry != "China" || a.LegalUnit != "千克" {
		t.Fatalf("要素未被快照：%+v", a)
	}

	// 改产品不该影响已快照的那一行：这是「快照」与「引用」的分界。
	p.HSCode = "9999"
	if a.HSCode != "8302" {
		t.Errorf("产品改码后分配行应保持原值，得到 %q", a.HSCode)
	}

	// 分配行上已经填过的不覆盖：分票申报时人会按实际用途改 HS，
	// 那种改动比产品上的默认值更贴近实际。
	b := &model.ShipmentAllocation{HSCode: "1111", OriginCountry: "Vietnam"}
	snapshotCustoms(b, p)
	if b.HSCode != "1111" || b.OriginCountry != "Vietnam" {
		t.Errorf("人工填过的要素不应被产品覆盖，得到 %q / %q", b.HSCode, b.OriginCountry)
	}

	// 报关明细由分配行投影，数量按定点字符串透传。
	alloc := model.ShipmentAllocation{
		SKUCode: "BH01-0001", NameCN: "合页", Qty: money.FromInt(120),
		Unit: "PCS", HSCode: "8302", OriginCountry: "China",
	}
	items := customsItems([]model.ShipmentAllocation{alloc})
	if len(items) != 1 || items[0].HSCode != "8302" || items[0].Qty != "120" {
		t.Fatalf("报关明细投影不正确：%+v", items)
	}
}
