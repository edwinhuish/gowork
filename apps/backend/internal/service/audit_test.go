package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

// auditFixture 用一个可变的时钟造"不同时间发生的操作"：
// 时间过滤与"To 含当天"这条最容易写错，而它只能靠跨天数据才测得出来。
type auditFixture struct {
	svc AuditService
	now time.Time
}

func newAuditFixture(t *testing.T) *auditFixture {
	t.Helper()
	db := openTestDB(t, &model.AuditLog{})
	f := &auditFixture{now: time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local)}
	f.svc = NewAuditService(
		repository.NewAuditLogRepository(db),
		WithAuditClock(func() time.Time { return f.now }),
	)
	return f
}

func (f *auditFixture) record(
	t *testing.T, at time.Time, actorID uint, actor, entity, action string, entityID uint, body string,
) {
	t.Helper()
	f.now = at
	err := f.svc.Record(context.Background(), dto.AuditEntry{
		ActorID: actorID, ActorName: actor, ActorRole: "user",
		Method: "PUT", Action: action, Entity: entity, EntityID: entityID,
		Path: "/api/v1/" + entity, Route: "/api/v1/" + entity + "/:id",
		Status: 200, RequestID: "rid", IP: "127.0.0.1", Body: body,
	})
	if err != nil {
		t.Fatalf("写审计失败: %v", err)
	}
}

func (f *auditFixture) list(t *testing.T, q dto.AuditLogQuery) []model.AuditLog {
	t.Helper()
	res, err := f.svc.List(context.Background(), q)
	if err != nil {
		t.Fatalf("查审计失败: %v", err)
	}
	return res.List
}

func TestAuditListOrderAndPaging(t *testing.T) {
	f := newAuditFixture(t)
	f.record(t, time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local), 1, "alice", "sales-orders", "status", 10, `{"status":20}`)
	f.record(t, time.Date(2026, 10, 2, 9, 0, 0, 0, time.Local), 2, "bob", "sales-orders", "update", 11, `{"remark":"改备注"}`)
	f.record(t, time.Date(2026, 10, 3, 9, 0, 0, 0, time.Local), 1, "alice", "partners", "merge", 5, `{"source_id":9}`)

	logs := f.list(t, dto.AuditLogQuery{})
	if len(logs) != 3 {
		t.Fatalf("应有 3 条，得到 %d", len(logs))
	}
	// 最近的在最上面：翻日志的人先看刚发生了什么。
	if logs[0].Entity != "partners" || logs[2].Entity != "sales-orders" {
		t.Errorf("应按 id 倒序，得到 %s ... %s", logs[0].Entity, logs[2].Entity)
	}

	q := dto.AuditLogQuery{PageQuery: dto.PageQuery{Page: 1, PageSize: 2}}
	res, err := f.svc.List(context.Background(), q)
	if err != nil {
		t.Fatalf("查审计失败: %v", err)
	}
	if len(res.List) != 2 || res.Total != 3 {
		t.Errorf("分页应为 2 条 / 合计 3，得到 %d / %d", len(res.List), res.Total)
	}
}

func TestAuditListFilters(t *testing.T) {
	f := newAuditFixture(t)
	f.record(t, time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local), 1, "alice", "sales-orders", "status", 10, `{"status":20}`)
	f.record(t, time.Date(2026, 10, 2, 9, 0, 0, 0, time.Local), 2, "bob", "sales-orders", "update", 11, `{"remark":"DZ2603-SO01 改交期"}`)
	f.record(t, time.Date(2026, 10, 3, 9, 0, 0, 0, time.Local), 1, "alice", "partners", "merge", 5, `{"source_id":9}`)

	t.Run("按资源", func(t *testing.T) {
		if got := f.list(t, dto.AuditLogQuery{Entity: "sales-orders"}); len(got) != 2 {
			t.Errorf("应有 2 条订单操作，得到 %d", len(got))
		}
	})
	t.Run("按动作", func(t *testing.T) {
		if got := f.list(t, dto.AuditLogQuery{Action: "merge"}); len(got) != 1 {
			t.Errorf("应有 1 条合并，得到 %d", len(got))
		}
	})
	t.Run("按单据 id", func(t *testing.T) {
		id := uint(11)
		got := f.list(t, dto.AuditLogQuery{Entity: "sales-orders", EntityID: &id})
		if len(got) != 1 || got[0].ActorName != "bob" {
			t.Errorf("应有 1 条且是 bob 改的，得到 %+v", got)
		}
	})
	t.Run("按操作人", func(t *testing.T) {
		actor := uint(1)
		if got := f.list(t, dto.AuditLogQuery{ActorID: &actor}); len(got) != 2 {
			t.Errorf("alice 应有 2 条，得到 %d", len(got))
		}
	})
	t.Run("按单号搜请求体", func(t *testing.T) {
		got := f.list(t, dto.AuditLogQuery{Keyword: "DZ2603-SO01"})
		if len(got) != 1 {
			t.Errorf("应搜到 1 条，得到 %d", len(got))
		}
	})
}

// To 是某天零点时，闭区间必须包住那一整天——否则当天的操作永远查不到。
func TestAuditListDateRangeIncludesWholeDay(t *testing.T) {
	f := newAuditFixture(t)
	f.record(t, time.Date(2026, 10, 1, 8, 0, 0, 0, time.Local), 1, "alice", "products", "create", 0, `{}`)
	f.record(t, time.Date(2026, 10, 2, 23, 30, 0, 0, time.Local), 1, "alice", "products", "update", 3, `{}`)
	f.record(t, time.Date(2026, 10, 3, 8, 0, 0, 0, time.Local), 1, "alice", "products", "update", 4, `{}`)

	got := f.list(t, dto.AuditLogQuery{From: "2026-10-02", To: "2026-10-02"})
	if len(got) != 1 || got[0].EntityID != 3 {
		t.Fatalf("应只查到 10-02 那条（含当天 23:30），得到 %+v", got)
	}
}

func TestAuditListRejectsBadDate(t *testing.T) {
	f := newAuditFixture(t)
	_, err := f.svc.List(context.Background(), dto.AuditLogQuery{From: "2026/10/01"})
	if err == nil {
		t.Fatal("日期格式不对时应报错")
	}
}
