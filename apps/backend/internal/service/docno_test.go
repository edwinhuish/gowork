package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

func newDocNoService(t *testing.T, clock func() time.Time) DocNoService {
	t.Helper()
	db := openTestDB(t, &model.DocSeq{})
	return NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock))
}

// 并发取号不重号 —— 这是单号生成唯一真正难的地方。
//
// 100 个协程同时要号：串行实现（查最大号 +1）在这里会大面积撞号，
// 而撞号在线上表现为"保存失败"，用户重试一次就过去了，于是没人报这个 bug。
func TestDocNoConcurrent(t *testing.T) {
	svc := newDocNoService(t, func() time.Time {
		return time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	})

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)

	docNos := make([]string, n)
	errsSeen := make([]error, n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			docNos[i], errsSeen[i] = svc.Next(context.Background(), model.DocTypePurchaseOrder)
		}(i)
	}
	close(start)
	wg.Wait()

	seen := make(map[string]struct{}, n)
	for i, docNo := range docNos {
		if errsSeen[i] != nil {
			t.Fatalf("第 %d 次取号失败: %v", i, errsSeen[i])
		}
		if _, dup := seen[docNo]; dup {
			t.Fatalf("单号重复: %s", docNo)
		}
		seen[docNo] = struct{}{}
	}
	if len(seen) != n {
		t.Fatalf("单号数量 = %d，期望 %d", len(seen), n)
	}
}

func TestDocNoFormatAndRollover(t *testing.T) {
	now := time.Date(2026, 10, 31, 23, 0, 0, 0, time.Local)
	svc := newDocNoService(t, func() time.Time { return now })
	ctx := context.Background()

	first, err := svc.Next(ctx, model.DocTypePurchaseOrder)
	if err != nil {
		t.Fatalf("取号失败: %v", err)
	}
	if first != "PO-202610-0001" {
		t.Errorf("首个单号 = %s，期望 PO-202610-0001", first)
	}

	second, err := svc.Next(ctx, model.DocTypePurchaseOrder)
	if err != nil {
		t.Fatalf("取号失败: %v", err)
	}
	if second != "PO-202610-0002" {
		t.Errorf("第二个单号 = %s，期望 PO-202610-0002", second)
	}

	// 类型不同就是不同的号段，互不影响。
	inbound, err := svc.Next(ctx, model.DocTypeStockIn)
	if err != nil {
		t.Fatalf("取号失败: %v", err)
	}
	if inbound != "IN-202610-0001" {
		t.Errorf("入库单号 = %s，期望 IN-202610-0001", inbound)
	}

	// 跨月重置：月初的单号从 0001 重新开始，而不是接着上个月。
	now = time.Date(2026, 11, 1, 0, 5, 0, 0, time.Local)
	nextMonth, err := svc.Next(ctx, model.DocTypePurchaseOrder)
	if err != nil {
		t.Fatalf("取号失败: %v", err)
	}
	if nextMonth != "PO-202611-0001" {
		t.Errorf("跨月单号 = %s，期望 PO-202611-0001", nextMonth)
	}
}

// 报价与订单不许从月号段发号：两条发号路径同时存在，同一张表里迟早长出两种格式。
func TestNextRejectsDealTypes(t *testing.T) {
	svc := newDocNoService(t, time.Now)
	for _, typ := range []model.DocType{model.DocTypeQuotation, model.DocTypeSalesOrder} {
		if _, err := svc.Next(context.Background(), typ); err == nil {
			t.Errorf("%s 应该被拒，改走 NextDeal", typ)
		}
	}
}

// 案号是「客户 + 年 + 第几笔生意」：同一客户次年从 01 重来，换客户各算各的。
func TestDealNoFormat(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	svc := newDocNoService(t, func() time.Time { return now })
	ctx := context.Background()

	key := DealKey{PartnerID: 7, PartnerCode: "DZ", Year: 2026}
	first, err := svc.NextDeal(ctx, key, model.DocTypeQuotation)
	if err != nil {
		t.Fatalf("发案号失败: %v", err)
	}
	if first.BaseNo != "DZ2601" || first.DocNo != "DZ2601-QT01" || first.Seq != 1 {
		t.Fatalf("首个案号 = %+v，期望 DZ2601 / DZ2601-QT01 / 1", first)
	}

	// 同一客户第二笔生意：案号加一，单号后缀跟着案号序号走。
	second, err := svc.NextDeal(ctx, key, model.DocTypeQuotation)
	if err != nil {
		t.Fatalf("发案号失败: %v", err)
	}
	if second.DocNo != "DZ2602-QT02" {
		t.Errorf("第二笔单号 = %s，期望 DZ2602-QT02", second.DocNo)
	}

	// 次年从 01 重来。
	key.Year = 2027
	nextYear, err := svc.NextDeal(ctx, key, model.DocTypeSalesOrder)
	if err != nil {
		t.Fatalf("发案号失败: %v", err)
	}
	if nextYear.BaseNo != "DZ2701" {
		t.Errorf("次年案号 = %s，期望 DZ2701", nextYear.BaseNo)
	}

	// 换客户各算各的：号段键带 PartnerID。
	other, err := svc.NextDeal(ctx, DealKey{PartnerID: 8, PartnerCode: "AB", Year: 2026},
		model.DocTypeQuotation)
	if err != nil {
		t.Fatalf("发案号失败: %v", err)
	}
	if other.BaseNo != "AB2601" {
		t.Errorf("另一客户的案号 = %s，期望 AB2601", other.BaseNo)
	}
}

// 序号两位用尽就报错，不扩位：单号是印在纸上的分段格式，多一位就错位。
func TestDealNoExhausted(t *testing.T) {
	svc := newDocNoService(t, func() time.Time {
		return time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	})
	ctx := context.Background()
	key := DealKey{PartnerID: 1, PartnerCode: "DZ", Year: 2026}

	for i := 1; i <= dealSeqMax; i++ {
		if _, err := svc.NextDeal(ctx, key, model.DocTypeQuotation); err != nil {
			t.Fatalf("第 %d 次发案号失败: %v", i, err)
		}
	}
	_, err := svc.NextDeal(ctx, key, model.DocTypeQuotation)
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
		t.Fatalf("号段用尽期望 40901，实际 %v", err)
	}
}

func TestDealNoRejectsOtherDocTypes(t *testing.T) {
	svc := newDocNoService(t, time.Now)
	_, err := svc.NextDeal(context.Background(),
		DealKey{PartnerID: 1, PartnerCode: "DZ", Year: 2026}, model.DocTypePurchaseOrder)
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
		t.Fatalf("采购单发案号期望 40001，实际 %v", err)
	}
}

// ParseDealDocNo 是转单时复制序号的入口：拆错了订单会拿到另一个序号，
// 表现是「QT03 转出 SO04」，两张单再也不是同一笔生意的两面。
func TestParseDealDocNo(t *testing.T) {
	cases := []struct {
		docNo  string
		typ    model.DocType
		base   string
		seq    uint
		wantOK bool
	}{
		{"DZ2603-QT01", model.DocTypeQuotation, "DZ2603", 1, true},
		{"DZ-CO2603-QT12", model.DocTypeQuotation, "DZ-CO2603", 12, true},
		// 类型不匹配：拿报价单号去解析订单序号必须失败，而不是"差不多就返回"。
		{"DZ2603-QT01", model.DocTypeSalesOrder, "", 0, false},
		// 旧格式（T-47 之前）没有案号，解析要老实失败，由调用方决定怎么补。
		{"QT-202610-0001", model.DocTypeQuotation, "", 0, false},
		{"DZ2603-QT00", model.DocTypeQuotation, "", 0, false},
	}
	for _, c := range cases {
		base, seq, ok := model.ParseDealDocNo(c.docNo, c.typ)
		if ok != c.wantOK || base != c.base || seq != c.seq {
			t.Errorf("ParseDealDocNo(%q, %s) = (%q, %d, %v)，期望 (%q, %d, %v)",
				c.docNo, c.typ, base, seq, ok, c.base, c.seq, c.wantOK)
		}
	}
}

func TestDocNoRejectsUnknownType(t *testing.T) {
	svc := newDocNoService(t, time.Now)

	_, err := svc.Next(context.Background(), model.DocType("XX"))
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
		t.Fatalf("期望 40001，实际 %v", err)
	}
}

func TestDocumentStatusTransit(t *testing.T) {
	cases := []struct {
		from, to model.DocumentStatus
		want     bool
	}{
		{model.DocumentStatusDraft, model.DocumentStatusConfirmed, true},
		{model.DocumentStatusDraft, model.DocumentStatusCancelled, true},
		{model.DocumentStatusDraft, model.DocumentStatusPreparing, false},
		{model.DocumentStatusDraft, model.DocumentStatusDraft, false},
		{model.DocumentStatusConfirmed, model.DocumentStatusPreparing, true},
		{model.DocumentStatusConfirmed, model.DocumentStatusDraft, false},
		{model.DocumentStatusPreparing, model.DocumentStatusShipped, true},
		{model.DocumentStatusPreparing, model.DocumentStatusCancelled, true},
		{model.DocumentStatusShipped, model.DocumentStatusCompleted, true},
		// 已出货不能取消：货已经在海上，要冲出得走退货单，不是改状态。
		{model.DocumentStatusShipped, model.DocumentStatusCancelled, false},
		{model.DocumentStatusCompleted, model.DocumentStatusCancelled, false},
		{model.DocumentStatusCancelled, model.DocumentStatusDraft, false},
		{model.DocumentStatusDraft, model.DocumentStatus(99), false},
	}

	for _, tc := range cases {
		if got := tc.from.CanTransitTo(tc.to); got != tc.want {
			t.Errorf("%s → %s = %v，期望 %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestCheckDocumentTransit(t *testing.T) {
	if err := checkDocumentTransit(model.DocumentStatusDraft, model.DocumentStatusConfirmed); err != nil {
		t.Fatalf("合法跳转被拒: %v", err)
	}

	err := checkDocumentTransit(model.DocumentStatusDraft, model.DocumentStatusShipped)
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
		t.Fatalf("非法跳转期望 40901，实际 %v", err)
	}

	err = checkDocumentTransit(model.DocumentStatusDraft, model.DocumentStatus(99))
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
		t.Fatalf("未知状态期望 40001，实际 %v", err)
	}
}
