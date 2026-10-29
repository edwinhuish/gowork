package service

import (
	"strings"
	"testing"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// TestOrderPDFPieces 覆盖订单打印件的两块纯逻辑：状态文案与明细行。
//
// 不在这里渲染整张 PDF：渲染依赖中文字体，CI 镜像常常没有，
// 那部分由 pdf_test.go 的用例在字体存在时验证。这里保证的是
// 「印出去的字不是 10 / 20 这种内部码」以及「明细行带得出 SKU」。
func TestOrderPDFPieces(t *testing.T) {
	cases := map[model.DocumentStatus]string{
		model.DocumentStatusDraft:     "Draft",
		model.DocumentStatusConfirmed: "Confirmed",
		model.DocumentStatusPreparing: "In Production",
		model.DocumentStatusShipped:   "Shipped",
		model.DocumentStatusCompleted: "Completed",
		model.DocumentStatusCancelled: "Cancelled",
		model.DocumentStatus(99):      "—",
	}
	for status, want := range cases {
		if got := statusText(status); got != want {
			t.Errorf("状态 %d 应印 %q，得到 %q", status, want, got)
		}
	}

	items := salesOrderItems([]model.SalesOrderItem{
		{
			SKUCode: "BH01-0001", NameCN: "不锈钢合页", Spec: "100×75×3",
			Qty: money.FromInt(2), Unit: "PCS",
			UnitPrice: money.MustParse("1.5"), Amount: money.MustParse("3"),
		},
	}, 2)
	if len(items) != 1 {
		t.Fatalf("应有 1 行，得到 %d", len(items))
	}
	// 明细第一列是 SKU：装箱单与订单上都要能按 SKU 核对，只印品名会对不上客户的清单。
	if !strings.Contains(items[0]["description"], "BH01-0001") {
		t.Errorf("明细应带 SKU，得到 %q", items[0]["description"])
	}
	if items[0]["qty"] == "" || items[0]["unit_price"] == "" {
		t.Errorf("数量与单价不能为空：%+v", items[0])
	}
}
