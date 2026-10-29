package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

func newLeadTestService(t *testing.T, now time.Time) (LeadService, repository.PartnerRepository) {
	t.Helper()
	return buildLeadService(t, now, nil)
}

// buildLeadService 建一套线索服务。txs 传 nil 就用真实的事务管理器，
// 传别的（如提交必失败的包装）用来验转换流程的事务边界。
func buildLeadService(
	t *testing.T, now time.Time, txs repository.TxManager,
) (LeadService, repository.PartnerRepository) {
	t.Helper()
	db := openTestDB(t,
		&model.Partner{}, &model.Product{}, &model.Currency{}, &model.ExchangeRate{}, &model.Lead{})

	partnerRepo := repository.NewPartnerRepository(db)
	partnerSvc := NewPartnerService(
		partnerRepo, repository.NewProductRepository(db), repository.NewCurrencyRepository(db))
	if txs == nil {
		txs = repository.NewTxManager(db)
	}
	svc := NewLeadService(
		repository.NewLeadRepository(db),
		partnerSvc,
		partnerRepo,
		txs,
		WithLeadClock(func() time.Time { return now }),
	)
	return svc, partnerRepo
}

func TestLeadConversion(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	svc, partnerRepo := newLeadTestService(t, now)

	lead, err := svc.Create(ctx, dto.LeadInput{
		Company:     "Ferreteria El Sol",
		ContactName: "Carlos",
		Phone:       "+593 9 0000 0001",
		Email:       "carlos@elsol.ec",
		Country:     "Ecuador",
		Source:      "exhibition",
		Demand:      "问 4 寸不锈钢合页，首批 5000 只",
	})
	if err != nil {
		t.Fatalf("新建线索失败: %v", err)
	}
	// 小写入参归一化，否则按来源筛选会同时存在 exhibition 与 EXHIBITION。
	if lead.Source != model.LeadSourceExhibition {
		t.Errorf("source = %s，期望 EXHIBITION", lead.Source)
	}
	if lead.Status != model.LeadStatusNew {
		t.Errorf("status = %d，期望新建", lead.Status)
	}

	t.Run("不能直接把状态改成已转化", func(t *testing.T) {
		converted := int8(model.LeadStatusConverted)
		_, err := svc.Update(ctx, lead.ID, dto.LeadInput{
			Company: lead.Company, Source: string(lead.Source), Status: &converted,
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001（请用转化功能），实际 %v", err)
		}
	})

	t.Run("转化生成主体并回填", func(t *testing.T) {
		res, err := svc.Convert(ctx, lead.ID, dto.LeadConvertInput{Code: "CUS900"})
		if err != nil {
			t.Fatalf("转化失败: %v", err)
		}
		if res.Partner.Code != "CUS900" || res.Partner.Name != "Ferreteria El Sol" {
			t.Errorf("主体 = %s / %s，期望 CUS900 / Ferreteria El Sol",
				res.Partner.Code, res.Partner.Name)
		}
		// 线索上的联系方式要带过去，否则转化后还要手工重录一遍。
		if res.Partner.Phone != lead.Phone || res.Partner.Email != lead.Email {
			t.Errorf("联系方式未带过去: %s / %s", res.Partner.Phone, res.Partner.Email)
		}
		if res.Lead.Status != model.LeadStatusConverted {
			t.Errorf("线索状态 = %d，期望已转化", res.Lead.Status)
		}
		if res.Lead.PartnerID == nil || *res.Lead.PartnerID != res.Partner.ID {
			t.Error("线索未回填 partner_id")
		}
		if res.Lead.ConvertedAt == nil {
			t.Error("线索未写转化时间")
		}
		if res.Warning != "" {
			t.Errorf("首次转化不该有重复提示，实际 %q", res.Warning)
		}

		// 角色默认为客户。
		stored, err := partnerRepo.FindByID(ctx, res.Partner.ID)
		if err != nil {
			t.Fatalf("查主体失败: %v", err)
		}
		if !stored.Role.Has(model.PartnerRoleCustomer) {
			t.Errorf("转化出的主体角色 = %d，期望含客户", stored.Role)
		}
	})

	t.Run("不能重复转化", func(t *testing.T) {
		_, err := svc.Convert(ctx, lead.ID, dto.LeadConvertInput{Code: "CUS901"})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	t.Run("同名同国只提示不阻断", func(t *testing.T) {
		dup, err := svc.Create(ctx, dto.LeadInput{
			Company: "Ferreteria El Sol", Country: "Ecuador", Source: "REFERRAL",
		})
		if err != nil {
			t.Fatalf("新建线索失败: %v", err)
		}
		res, err := svc.Convert(ctx, dup.ID, dto.LeadConvertInput{Code: "CUS902"})
		if err != nil {
			t.Fatalf("同名线索转化不该被拒: %v", err)
		}
		if !strings.Contains(res.Warning, "CUS900") {
			t.Errorf("应提示已存在的同名主体 CUS900，实际 %q", res.Warning)
		}
	})

	t.Run("新建线索不能是已转化", func(t *testing.T) {
		converted := int8(model.LeadStatusConverted)
		_, err := svc.Create(ctx, dto.LeadInput{
			Company: "X", Source: "OTHER", Status: &converted,
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("已废弃可重新激活", func(t *testing.T) {
		dropped, err := svc.Create(ctx, dto.LeadInput{Company: "Y", Source: "GOOGLE"})
		if err != nil {
			t.Fatalf("新建线索失败: %v", err)
		}
		st := int8(model.LeadStatusDropped)
		if _, err := svc.Update(ctx, dropped.ID, dto.LeadInput{
			Company: "Y", Source: "GOOGLE", Status: &st,
		}); err != nil {
			t.Fatalf("新建→已废弃应被允许: %v", err)
		}
		following := int8(model.LeadStatusFollowing)
		back, err := svc.Update(ctx, dropped.ID, dto.LeadInput{
			Company: "Y", Source: "GOOGLE", Status: &following,
		})
		if err != nil {
			t.Fatalf("已废弃→跟进中应被允许（客户又回来了）: %v", err)
		}
		if back.Status != model.LeadStatusFollowing {
			t.Errorf("status = %d，期望跟进中", back.Status)
		}
	})
}

// 转化失败时不能留下孤儿主体：建主体与标记线索必须同生共死。
func TestLeadConvertRollsBackOnFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)

	db := openTestDB(t,
		&model.Partner{}, &model.Product{}, &model.Currency{}, &model.ExchangeRate{}, &model.Lead{})
	partnerRepo := repository.NewPartnerRepository(db)
	partnerSvc := NewPartnerService(
		partnerRepo, repository.NewProductRepository(db), repository.NewCurrencyRepository(db))
	svc := NewLeadService(
		repository.NewLeadRepository(db), partnerSvc, partnerRepo,
		failingTx{inner: repository.NewTxManager(db)},
		WithLeadClock(func() time.Time { return now }),
	)

	lead, err := svc.Create(ctx, dto.LeadInput{Company: "Rollback Ltd", Source: "GOOGLE"})
	if err != nil {
		t.Fatalf("新建线索失败: %v", err)
	}

	if _, err := svc.Convert(ctx, lead.ID, dto.LeadConvertInput{Code: "RB001"}); !errors.Is(err, errCommitFailed) {
		t.Fatalf("期望拿到提交失败的哨兵错误，实际 %v", err)
	}

	// 主体没留下、线索也没被标成已转化：两者都在同一个事务里。
	_, total, err := partnerRepo.List(ctx, repository.PartnerFilter{Keyword: "RB001"}, repository.Pagination{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("查主体失败: %v", err)
	}
	if total != 0 {
		t.Fatalf("回滚后不该留下主体，得到 %d 条", total)
	}
	reloaded, err := svc.Get(ctx, lead.ID)
	if err != nil {
		t.Fatalf("查线索失败: %v", err)
	}
	if reloaded.Status == model.LeadStatusConverted {
		t.Fatal("回滚后线索不该是已转化")
	}
}
