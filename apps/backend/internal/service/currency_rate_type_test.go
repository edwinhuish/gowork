package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// TestCurrencyRateType 是 T-60 的验收点。
//
// 两件事一起验：
//   - 同一天同一币种能同时存中间价与买入价（唯一索引已经含 rate_type，
//     否则第二次插入会撞唯一约束——这正是那次索引迁移要保证的事）；
//   - 取价按类型走，缺该类型时回退中间价。
//
// 没有卖出价是刻意的：存量数据只有中间价，回退路径必须被覆盖，
// 否则上线当天所有付款都会报"缺少汇率"。
func TestCurrencyRateType(t *testing.T) {
	cur, rates := newCurrencyTestService(t)
	ctx := context.Background()

	createCurrency(t, cur, "CNY", "人民币", true)
	createCurrency(t, cur, "USD", "美元", false)

	day := "2026-10-30"
	on := time.Date(2026, 10, 30, 10, 0, 0, 0, time.Local)

	addRate := func(rateType, rate string) {
		t.Helper()
		in := dto.ExchangeRateInput{
			CurrencyCode:  "USD",
			EffectiveDate: day,
			Rate:          money.MustParse(rate),
		}
		in.RateType = rateType
		if _, err := rates.Create(ctx, in); err != nil {
			t.Fatalf("录入 %s 汇率: %v", rateType, err)
		}
	}
	addRate(model.RateTypeMiddle, "7.10")
	addRate(model.RateTypeBuy, "7.05")

	// 收款：取买入价。
	got, rate, err := cur.ToBaseDecimalFor(ctx, money.FromInt(100), "USD", on, model.RateTypeBuy)
	if err != nil {
		t.Fatalf("按买入价折算失败: %v", err)
	}
	if rate.Cmp(money.MustParse("7.05")) != 0 {
		t.Errorf("买入价应为 7.05，得到 %s", rate)
	}
	if got.Cmp(money.MustParse("705")) != 0 {
		t.Errorf("100 美元按买入价应折 705，得到 %s", got)
	}

	// 付款：当天没有卖出价，回退中间价——不是报错。
	_, sellRate, err := cur.ToBaseDecimalFor(ctx, money.FromInt(100), "USD", on, model.RateTypeSell)
	if err != nil {
		t.Fatalf("缺卖出价时应回退中间价，实际报错: %v", err)
	}
	if sellRate.Cmp(money.MustParse("7.10")) != 0 {
		t.Errorf("缺卖出价应回退到 7.10，得到 %s", sellRate)
	}

	// 单据锁汇率仍走中间价：它不是任何一笔实际兑换的价格。
	_, midRate, err := cur.ToBaseDecimal(ctx, money.FromInt(1), "USD", on)
	if err != nil {
		t.Fatalf("取中间价失败: %v", err)
	}
	if midRate.Cmp(money.MustParse("7.10")) != 0 {
		t.Errorf("中间价应为 7.10，得到 %s", midRate)
	}

	// 本币不取价，恒为 1。
	_, baseRate, err := cur.ToBaseDecimalFor(ctx, money.FromInt(1), "CNY", on, model.RateTypeBuy)
	if err != nil {
		t.Fatalf("本币折算失败: %v", err)
	}
	if baseRate.Cmp(money.FromInt(1)) != 0 {
		t.Errorf("本币汇率应为 1，得到 %s", baseRate)
	}
}
