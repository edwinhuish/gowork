package repository

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/database"
	"github.com/edwinhuish/gowork/internal/model"
)

// txFixture 是一套最小的仓储 + 事务管理器，用来验事务边界本身。
type txFixture struct {
	txs       TxManager
	partners  PartnerRepository
	products  ProductRepository
	partnerID uint
}

func newTxFixture(t *testing.T) txFixture {
	t.Helper()
	ctx := context.Background()

	db, err := database.Open(config.Database{
		Driver: config.DriverSQLite,
		DSN:    filepath.Join(t.TempDir(), "tx.db"),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("连接 sqlite 失败: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	if err := database.AutoMigrate(db, &model.Partner{}, &model.Product{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	partners := NewPartnerRepository(db)
	supplier := &model.Partner{
		Code: "TX-SUP", Name: "事务测试供应商",
		Role: model.PartnerRoleSupplier,
	}
	if err := partners.Create(ctx, supplier); err != nil {
		t.Fatalf("建供应商: %v", err)
	}
	return txFixture{
		txs:       NewTxManager(db),
		partners:  partners,
		products:  NewProductRepository(db),
		partnerID: supplier.ID,
	}
}

func (f txFixture) countProducts(t *testing.T) int64 {
	t.Helper()
	n, err := f.products.Count(context.Background())
	if err != nil {
		t.Fatalf("统计产品: %v", err)
	}
	return n
}

func TestTxManagerRollsBackEveryRepo(t *testing.T) {
	ctx := context.Background()
	f := newTxFixture(t)
	before := f.countProducts(t)

	boom := errors.New("中途失败")
	err := f.txs.Transaction(ctx, func(ctx context.Context) error {
		// 两个不同的仓储写两张表，都该跟着这次回滚一起消失。
		if err := f.products.Create(ctx, &model.Product{
			SKUCode: "TX-001", NameCN: "事务产品",
			SupplierID: &f.partnerID,
		}); err != nil {
			return err
		}
		if err := f.partners.Update(ctx, f.partnerID, map[string]any{"name": "改过的名字"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("应原样返回业务错误，得到 %v", err)
	}

	if got := f.countProducts(t); got != before {
		t.Fatalf("回滚后不该留下产品行，期望 %d 条，得到 %d", before, got)
	}
	partner, err := f.partners.FindByID(ctx, f.partnerID)
	if err != nil {
		t.Fatalf("查供应商: %v", err)
	}
	if partner.Name == "改过的名字" {
		t.Fatalf("回滚后主体名不该被改掉")
	}
}

func TestTxManagerSharesOneConnection(t *testing.T) {
	ctx := context.Background()
	f := newTxFixture(t)
	before := f.countProducts(t)

	err := f.txs.Transaction(ctx, func(ctx context.Context) error {
		if err := f.products.Create(ctx, &model.Product{
			SKUCode: "TX-002", NameCN: "可见性产品",
			SupplierID: &f.partnerID,
		}); err != nil {
			return err
		}
		// 同事务内另一个仓储必须立刻看得见这行；看不见说明它拿的是池子连接。
		n, err := f.products.Count(ctx)
		if err != nil {
			return err
		}
		if n != before+1 {
			t.Fatalf("事务内应看得见刚写的行，期望 %d 条，得到 %d", before+1, n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("提交失败: %v", err)
	}

	if got := f.countProducts(t); got != before+1 {
		t.Fatalf("提交后应留下产品行，得到 %d 条", got)
	}
}

func TestTxManagerNestedJoinsOuter(t *testing.T) {
	ctx := context.Background()
	f := newTxFixture(t)
	before := f.countProducts(t)

	boom := errors.New("外层失败")
	err := f.txs.Transaction(ctx, func(ctx context.Context) error {
		// 仓储内部自己开的事务（如 CreateWithItems）会走到这里：
		// 它必须是 savepoint，而不是第二条连接上的新事务。
		if err := f.txs.Transaction(ctx, func(ctx context.Context) error {
			if err := f.products.Create(ctx, &model.Product{
				SKUCode: "TX-003", NameCN: "嵌套事务产品",
				SupplierID: &f.partnerID,
			}); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("应原样返回业务错误，得到 %v", err)
	}

	if got := f.countProducts(t); got != before {
		t.Fatalf("外层回滚必须连带撤掉内层的写入，得到 %d 条", got)
	}
}
