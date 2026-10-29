package service

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/database"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// openTestDB 起一个临时 sqlite 并建好给定的表。
//
// service 层的测试尽量跑在真实数据库上，而不是手写假仓储：
// 唯一索引、事务、写成一条 SQL 的自增，恰恰是假仓储会顺手抹掉的东西，
// 而并发重号、号段不跳、重复汇率这些问题全都长在那里。
func openTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()

	db, err := database.Open(config.Database{
		Driver:      config.DriverSQLite,
		DSN:         filepath.Join(t.TempDir(), "test.db"),
		MaxOpen:     5,
		MaxIdle:     2,
		MaxLifetime: 60,
		LogLevel:    "error",
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("连接 sqlite 失败: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	if err := database.AutoMigrate(db, models...); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return db
}

// wantDecimal 断言定点金额等于给定字面量。
//
// 定点之后不再有"差一点点算过"：解析出来的表示是精确的整数单位，
// 比到末位才是它存在的理由。
func wantDecimal(t *testing.T, label string, got money.Decimal, want string) {
	t.Helper()
	w := money.MustParse(want)
	if got.Cmp(w) != 0 {
		t.Errorf("%s 应为 %s，得到 %s", label, w, got)
	}
}

// errCommitFailed 是"事务提交阶段失败"的哨兵值。
var errCommitFailed = errors.New("模拟事务提交失败")

// failingTx 让整笔事务在 fn 成功之后仍然回滚。
//
// 用它验的是"这段流程真的在一个事务里"：只要中间有一步走的是池子连接，
// 那条写入就撤不回来，断言会当场失败。比构造并发更可控，也不用 sleep。
type failingTx struct{ inner repository.TxManager }

func (f failingTx) Transaction(ctx context.Context, fn func(context.Context) error) error {
	return f.inner.Transaction(ctx, func(ctx context.Context) error {
		if err := fn(ctx); err != nil {
			return err
		}
		return errCommitFailed
	})
}

// qtyOf 是测试里构造数量的简写：数量也是定点数，字面量写成 qtyOf(100)。
func qtyOf(v float64) money.Decimal { return money.FromFloat(v) }

// approxQty 判断定点数量是否等于给定字面量。数量存 3 位小数，来回一趟不该有尾差，
// 所以这里不留容差——留了就等于允许"少发一件也过"。
func approxQty(a money.Decimal, want float64) bool {
	return a.Cmp(money.FromFloat(want)) == 0
}
