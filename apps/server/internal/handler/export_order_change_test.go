package handler

import (
	"testing"
	"time"

	"gowork_server/internal/model"
	"gowork_server/internal/pkg/money"
)

// 变更历史的导出行：一次改两行 → 两行；只改交期 → 一行占位。
//
// 占位那行是关键：只改交期的变更没有明细行，若不占位，这次变更在文件里
// 就消失了 —— 而它同样改变了订单，审计时"改过三次只见到两次"没法解释。
func TestOrderChangeRows(t *testing.T) {
	created := time.Date(2026, 11, 9, 10, 30, 0, 0, time.Local)
	before := time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local)
	after := time.Date(2026, 11, 25, 0, 0, 0, 0, time.Local)

	list := []model.SalesOrderChange{
		{
			Seq:         1,
			DocNo:       "SO-1-CH1",
			Base:        model.Base{CreatedAt: created},
			Operator:    "admin",
			Reason:      "客户减量",
			BeforeTotal: money.MustParse("1000"),
			AfterTotal:  money.MustParse("800"),
			Lines: []model.SalesOrderChangeLine{
				{
					SKUCode:         "PK01-0001",
					BeforeQty:       money.MustParse("100"),
					AfterQty:        money.MustParse("80"),
					BeforeUnitPrice: money.MustParse("10"),
					AfterUnitPrice:  money.MustParse("10"),
				},
			},
		},
		{
			Seq:                2,
			DocNo:              "SO-1-CH2",
			Base:               model.Base{CreatedAt: created},
			Operator:           "admin",
			Reason:             "客户要求推迟",
			BeforeTotal:        money.MustParse("800"),
			AfterTotal:         money.MustParse("800"),
			BeforeDeliveryDate: &before,
			AfterDeliveryDate:  &after,
		},
	}

	rows := orderChangeRows(list)

	if len(rows) != 2 {
		t.Fatalf("行数 = %d，期望 2（一次带明细 + 一次只改交期）", len(rows))
	}
	for i, row := range rows {
		if len(row) != len(orderChangeHeader) {
			t.Fatalf("第 %d 行列数 = %d，表头 %d 列；错位会让整份文件读不出来", i, len(row), len(orderChangeHeader))
		}
	}

	// 第一行：带明细的那次，SKU 与前后数量都在。
	if got := rows[0][5]; got != "PK01-0001" {
		t.Errorf("第一行 SKU = %q，期望 PK01-0001", got)
	}
	if got := rows[0][6]; got != "100" {
		t.Errorf("第一行原数量 = %q，期望 100", got)
	}
	// 第二行：只改交期，明细列留空而不是 0 —— 0 会被读成"改成 0 了"。
	if got := rows[1][5]; got != "" {
		t.Errorf("占位行 SKU = %q，期望空", got)
	}
	if got := rows[1][6]; got != "" {
		t.Errorf("占位行原数量 = %q，期望空", got)
	}
	if got := rows[1][12]; got != "2026-11-20" {
		t.Errorf("原交期 = %q，期望 2026-11-20", got)
	}
	if got := rows[1][13]; got != "2026-11-25" {
		t.Errorf("新交期 = %q，期望 2026-11-25", got)
	}
}
