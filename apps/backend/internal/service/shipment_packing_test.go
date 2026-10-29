package service

import (
	"testing"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// TestFillPackingFromProduct 是 T-50 的验收点：产品维护了箱规之后，
// 出货分配行的箱数 / 毛净重 / 体积由「箱数 × 单箱值」推出，不再人工算。
//
// 四条规则里最要紧的是第二条：人工填了就不覆盖。
// 同一票货混装、临时加箱是常态，用产品规格盖上真实的装箱数据，
// 印出去的装箱单就是错的，而错在报关与订舱两侧都会被退回来。
func TestFillPackingFromProduct(t *testing.T) {
	p := &model.Product{
		PackQty:           20,
		CartonGrossWeight: money.MustParse("12"),
		CartonNetWeight:   money.MustParse("11.5"),
		CartonVolume:      money.MustParse("0.072"),
		CartonSize:        "60×40×30CM",
	}

	// ① 整行没填：1000 件、每箱 20 件 → 50 箱 / 600kg / 575kg / 3.6m³
	a := &model.ShipmentAllocation{Qty: money.FromInt(1000)}
	fillPackingFromProduct(a, p)
	if a.Cartons != 50 {
		t.Errorf("箱数应为 50，得到 %d", a.Cartons)
	}
	if a.GrossWeight.Cmp(money.MustParse("600")) != 0 {
		t.Errorf("毛重应为 600，得到 %s", a.GrossWeight)
	}
	if a.NetWeight.Cmp(money.MustParse("575")) != 0 {
		t.Errorf("净重应为 575，得到 %s", a.NetWeight)
	}
	if a.Volume.Cmp(money.MustParse("3.6")) != 0 {
		t.Errorf("体积应为 3.6，得到 %s", a.Volume)
	}
	if a.Measurement != "60×40×30CM" {
		t.Errorf("外箱尺寸应取产品上的值，得到 %q", a.Measurement)
	}

	// ② 有余数向上取整：1001 件仍是 51 箱——半箱货也占一个箱子，
	// 订舱不会因为没装满就少算体积。
	b := &model.ShipmentAllocation{Qty: money.FromInt(1001)}
	fillPackingFromProduct(b, p)
	if b.Cartons != 51 {
		t.Errorf("有余数应向上取整为 51 箱，得到 %d", b.Cartons)
	}

	// ③ 人工填了任意一项装箱数据：整行都不动。
	manual := &model.ShipmentAllocation{
		Qty: money.FromInt(1000), Cartons: 48, GrossWeight: money.MustParse("580"),
	}
	fillPackingFromProduct(manual, p)
	if manual.Cartons != 48 || manual.GrossWeight.Cmp(money.MustParse("580")) != 0 {
		t.Errorf("人工填过的行不应被规格覆盖，得到 %d 箱 / %s kg", manual.Cartons, manual.GrossWeight)
	}
	if !manual.Volume.IsZero() {
		t.Errorf("人工填过的行不应凭空得到体积，得到 %s", manual.Volume)
	}

	// ④ 产品没维护箱规：什么都不做，不能让装箱数据变成 0。
	bare := &model.ShipmentAllocation{Qty: money.FromInt(1000)}
	fillPackingFromProduct(bare, &model.Product{PackQty: 0})
	if bare.Cartons != 0 || !bare.GrossWeight.IsZero() {
		t.Errorf("产品没有箱规时不应改动分配行，得到 %d 箱 / %s kg", bare.Cartons, bare.GrossWeight)
	}
}
