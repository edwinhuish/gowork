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
			docNos[i], errsSeen[i] = svc.Next(context.Background(), model.DocTypeSalesOrder)
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

	first, err := svc.Next(ctx, model.DocTypeSalesOrder)
	if err != nil {
		t.Fatalf("取号失败: %v", err)
	}
	if first != "SO-202610-0001" {
		t.Errorf("首个单号 = %s，期望 SO-202610-0001", first)
	}

	second, err := svc.Next(ctx, model.DocTypeSalesOrder)
	if err != nil {
		t.Fatalf("取号失败: %v", err)
	}
	if second != "SO-202610-0002" {
		t.Errorf("第二个单号 = %s，期望 SO-202610-0002", second)
	}

	// 类型不同就是不同的号段，互不影响。
	quotation, err := svc.Next(ctx, model.DocTypeQuotation)
	if err != nil {
		t.Fatalf("取号失败: %v", err)
	}
	if quotation != "QT-202610-0001" {
		t.Errorf("报价单号 = %s，期望 QT-202610-0001", quotation)
	}

	// 跨月重置：月初的单号从 0001 重新开始，而不是接着上个月。
	now = time.Date(2026, 11, 1, 0, 5, 0, 0, time.Local)
	nextMonth, err := svc.Next(ctx, model.DocTypeSalesOrder)
	if err != nil {
		t.Fatalf("取号失败: %v", err)
	}
	if nextMonth != "SO-202611-0001" {
		t.Errorf("跨月单号 = %s，期望 SO-202611-0001", nextMonth)
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
