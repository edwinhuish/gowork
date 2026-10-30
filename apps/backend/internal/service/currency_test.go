package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// 币种与折算的测试跑在真实 sqlite 上，而不是手写假仓储。
//
// Convert 的错最容易出在"日期怎么比"与"唯一索引命不命中"这两处，
// 假仓储把这两件事都绕过去了，测出来的只是 if 分支。
func newCurrencyTestService(t *testing.T) (CurrencyService, ExchangeRateService) {
	t.Helper()

	db := openTestDB(t, &model.Currency{}, &model.ExchangeRate{})
	currencyRepo := repository.NewCurrencyRepository(db)
	rateRepo := repository.NewExchangeRateRepository(db)
	return NewCurrencyService(currencyRepo, rateRepo), NewExchangeRateService(rateRepo, currencyRepo)
}

func createCurrency(t *testing.T, svc CurrencyService, code, name string, isBase bool) *model.Currency {
	t.Helper()
	m, err := svc.Create(context.Background(), dto.CurrencyInput{
		Code:   code,
		Name:   name,
		IsBase: &isBase,
	})
	if err != nil {
		t.Fatalf("创建币种 %s: %v", code, err)
	}
	return m
}

func createRate(t *testing.T, svc ExchangeRateService, code, rate string, on time.Time) {
	t.Helper()
	_, err := svc.Create(context.Background(), dto.ExchangeRateInput{
		CurrencyCode:  code,
		Rate:          money.MustParse(rate),
		EffectiveDate: on.Format(rateDateLayout),
	})
	if err != nil {
		t.Fatalf("登记 %s 在 %s 的汇率: %v", code, on.Format(rateDateLayout), err)
	}
}

func TestConvert(t *testing.T) {
	ctx := context.Background()
	currencySvc, rateSvc := newCurrencyTestService(t)
	on := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)

	createCurrency(t, currencySvc, "CNY", "人民币", true)
	createCurrency(t, currencySvc, "USD", "美元", false)
	createCurrency(t, currencySvc, "EUR", "欧元", false)
	createRate(t, rateSvc, "USD", "7.10", on)
	createRate(t, rateSvc, "EUR", "7.75", on)

	cases := []struct {
		name     string
		amount   string
		from, to string
		want     string
		wantRate string
	}{
		{name: "外币折本币", amount: "100", from: "USD", to: "CNY", want: "710", wantRate: "7.1"},
		// 1/7.1 = 0.140845... 定点保留 6 位，710 折回去正好是 100.00。
		{name: "本币折外币", amount: "710", from: "CNY", to: "USD", want: "100", wantRate: "0.140845"},
		// 交叉汇率同样保留 6 位：7.1/7.75 = 0.9161290... 取 0.916129。
		{name: "两个外币走交叉汇率", amount: "100", from: "USD", to: "EUR", want: "91.61", wantRate: "0.916129"},
		{name: "同币种原样返回", amount: "123.456", from: "USD", to: "USD", want: "123.456", wantRate: "1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, rate, err := currencySvc.ConvertDecimal(
				ctx, money.MustParse(tc.amount), tc.from, tc.to, on)
			if err != nil {
				t.Fatalf("折算失败: %v", err)
			}
			wantDecimal(t, "折算金额", got, tc.want)
			wantDecimal(t, "汇率", rate, tc.wantRate)
		})
	}
}

// 生效日之后的日期沿用最近一条汇率：录入一次价不必每天补。
func TestConvertUsesLatestEffectiveRate(t *testing.T) {
	ctx := context.Background()
	currencySvc, rateSvc := newCurrencyTestService(t)
	rateDate := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	createCurrency(t, currencySvc, "CNY", "人民币", true)
	createCurrency(t, currencySvc, "USD", "美元", false)
	createRate(t, rateSvc, "USD", "7.0", rateDate)

	got, _, err := currencySvc.ConvertDecimal(
		ctx, money.MustParse("10"), "USD", "CNY", rateDate.AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("折算失败: %v", err)
	}
	wantDecimal(t, "折算金额", got, "70")
}

// 缺汇率要报 400 而不是 500：这是"还没录数据"，不是服务出错。
func TestConvertWithoutRate(t *testing.T) {
	ctx := context.Background()
	currencySvc, rateSvc := newCurrencyTestService(t)
	on := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)

	createCurrency(t, currencySvc, "CNY", "人民币", true)
	createCurrency(t, currencySvc, "USD", "美元", false)
	createRate(t, rateSvc, "USD", "7.1", on)

	_, _, err := currencySvc.ConvertDecimal(ctx, money.MustParse("100"), "USD", "CNY", on.AddDate(0, 0, -1))
	if err == nil {
		t.Fatal("期望缺少汇率时报错")
	}
	e, ok := errs.From(err)
	if !ok || e.Code != errs.CodeInvalidParam {
		t.Fatalf("期望 40001，实际 %v", err)
	}
}

func TestCurrencyRules(t *testing.T) {
	ctx := context.Background()
	currencySvc, rateSvc := newCurrencyTestService(t)
	on := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)

	base := createCurrency(t, currencySvc, "CNY", "人民币", true)
	createCurrency(t, currencySvc, "USD", "美元", false)
	createRate(t, rateSvc, "USD", "7.1", on)

	t.Run("本币不能删除", func(t *testing.T) {
		err := currencySvc.Delete(ctx, base.ID)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	t.Run("有汇率的币种只能归档", func(t *testing.T) {
		usd, err := currencySvc.List(ctx, dto.CurrencyQuery{PageQuery: dto.PageQuery{Keyword: "USD"}})
		if err != nil || len(usd.List) != 1 {
			t.Fatalf("查 USD 失败: %v", err)
		}
		err = currencySvc.Delete(ctx, usd.List[0].ID)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	t.Run("没有汇率的币种删掉后能恢复", func(t *testing.T) {
		jpy, err := currencySvc.Create(ctx, dto.CurrencyInput{Code: "JPY", Name: "日元", Symbol: "¥"})
		if err != nil {
			t.Fatalf("建 JPY 失败: %v", err)
		}
		if err := currencySvc.Delete(ctx, jpy.ID); err != nil {
			t.Fatalf("删除 JPY 失败: %v", err)
		}
		if _, err := currencySvc.Get(ctx, jpy.ID); !errors.Is(err, errs.ErrNotFound) {
			t.Errorf("删掉的币种 Get 应 404，得到 %v", err)
		}
		// 已删除的币种只在「含已删除」里出现，界面上靠它点恢复。
		withDeleted, err := currencySvc.List(ctx, dto.CurrencyQuery{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("含已删除列表失败: %v", err)
		}
		found := false
		for _, row := range withDeleted.List {
			if row.ID == jpy.ID {
				found = true
			}
		}
		if !found {
			t.Error("含已删除时应查得到这条 JPY")
		}

		if _, err := currencySvc.Restore(ctx, jpy.ID); err != nil {
			t.Fatalf("恢复 JPY 失败: %v", err)
		}
		if _, err := currencySvc.Restore(ctx, jpy.ID); err == nil {
			t.Error("恢复未被删除的币种应报错")
		}
		// 恢复之后重新录一个同代码的，再恢复就成了冲突：
		// 单据按代码折算，两个同代码的行分不清该命中哪个。
		if err := currencySvc.Delete(ctx, jpy.ID); err != nil {
			t.Fatalf("再次删除失败: %v", err)
		}
		if _, err := currencySvc.Create(ctx, dto.CurrencyInput{Code: "JPY", Name: "日元", Symbol: "¥"}); err != nil {
			t.Fatalf("重建 JPY 失败: %v", err)
		}
		if _, err := currencySvc.Restore(ctx, jpy.ID); err == nil {
			t.Error("代码被占时应拒绝恢复")
		}
	})

	t.Run("同一天同一币种不能有两条汇率", func(t *testing.T) {
		_, err := rateSvc.Create(ctx, dto.ExchangeRateInput{
			CurrencyCode:  "USD",
			Rate:          money.MustParse("7.2"),
			EffectiveDate: on.Format(rateDateLayout),
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	t.Run("币种代码不可修改", func(t *testing.T) {
		usd, err := currencySvc.List(ctx, dto.CurrencyQuery{PageQuery: dto.PageQuery{Keyword: "USD"}})
		if err != nil || len(usd.List) != 1 {
			t.Fatalf("查 USD 失败: %v", err)
		}
		_, err = currencySvc.Update(ctx, usd.List[0].ID, dto.CurrencyInput{Code: "USX", Name: "美元"})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("本币相对自身不登记汇率", func(t *testing.T) {
		_, err := rateSvc.Create(ctx, dto.ExchangeRateInput{
			CurrencyCode:  "CNY",
			Rate:          money.FromInt(1),
			EffectiveDate: on.Format(rateDateLayout),
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	if _, ok := errs.From(errors.New("x")); ok {
		t.Fatal("非业务错误不应被识别为 errs.Error")
	}
}
