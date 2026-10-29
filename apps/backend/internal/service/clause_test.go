package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// 条款模板是主数据：编码唯一、能停用、能恢复。
func TestClauseTemplateCRUD(t *testing.T) {
	fx := newContractFixture(t, time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local))
	ctx := context.Background()

	created, err := fx.clauseSvc.Create(ctx, dto.ClauseInput{
		Code: "PAY-30", Title: "付款条件", Category: "付款",
		Body: "30% 订金于签订后 7 日内电汇，余款见提单副本后 7 日内付清。",
	})
	if err != nil {
		t.Fatalf("新增条款失败: %v", err)
	}
	if created.Status != model.EnableStatusOn {
		t.Errorf("不填状态应默认启用，得到 %v", created.Status)
	}

	t.Run("编码不能重复", func(t *testing.T) {
		_, err := fx.clauseSvc.Create(ctx, dto.ClauseInput{
			Code: "PAY-30", Title: "重复", Body: "x",
		})
		wantCode(t, err, errs.CodeConflict, "重复编码")
	})

	t.Run("正文不能为空", func(t *testing.T) {
		_, err := fx.clauseSvc.Create(ctx, dto.ClauseInput{Code: "X1", Title: "空正文", Body: "  "})
		wantCode(t, err, errs.CodeInvalidParam, "空正文")
	})

	t.Run("停用与恢复", func(t *testing.T) {
		off := int8(model.EnableStatusOff)
		updated, err := fx.clauseSvc.Update(ctx, created.ID, dto.ClauseInput{
			Code: "PAY-30", Title: "付款条件", Body: created.Body, Status: &off,
		})
		if err != nil {
			t.Fatalf("停用失败: %v", err)
		}
		if updated.Status != model.EnableStatusOff {
			t.Errorf("状态应为停用，得到 %v", updated.Status)
		}

		if err := fx.clauseSvc.Delete(ctx, created.ID); err != nil {
			t.Fatalf("删除失败: %v", err)
		}
		if _, err := fx.clauseSvc.Restore(ctx, created.ID); err != nil {
			t.Fatalf("恢复失败: %v", err)
		}
	})
}

// T-27 的验收点：新建合同时可选已有条款，**改动不影响**已生成的合同。
// 这不是靠校验守住的，是靠合同上存的是快照。
func TestContractClauseSnapshot(t *testing.T) {
	fx := newContractFixture(t, time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local))
	ctx := context.Background()

	pay, err := fx.clauseSvc.Create(ctx, dto.ClauseInput{
		Code: "PAY-30", Title: "付款条件", Category: "付款",
		Body: "30% 订金于签订后 7 日内电汇。",
	})
	if err != nil {
		t.Fatalf("建付款条款失败: %v", err)
	}
	qual, err := fx.clauseSvc.Create(ctx, dto.ClauseInput{
		Code: "QUAL-1", Title: "质量异议期", Category: "质量",
		Body: "到港后 30 日内提出异议，逾期视为验收合格。",
	})
	if err != nil {
		t.Fatalf("建质量条款失败: %v", err)
	}

	q := fx.confirmedQuotation(t)
	// 故意与创建顺序相反：合同上的顺序应当是**提交顺序**，不是模板的创建顺序。
	c, err := fx.svc.Generate(ctx, dto.ContractGenerateInput{
		Type:       int8(model.ContractTypePI),
		SourceType: int8(model.ContractSourceQuotation),
		SourceID:   q.ID,
		SignDate:   "2026-10-28",
		Terms: []dto.ContractTermInput{
			{Name: "全额", Amount: money.MustParse("1000")},
		},
		ClauseIDs: []uint{qual.ID, pay.ID, qual.ID}, // 末尾那条是重复选择，应被去重
	})
	if err != nil {
		t.Fatalf("生成合同失败: %v", err)
	}

	if len(c.Clauses) != 2 {
		t.Fatalf("应落 2 条条款快照（重复选择去重），得到 %d", len(c.Clauses))
	}
	if c.Clauses[0].Title != qual.Title || c.Clauses[0].Seq != 1 {
		t.Errorf("第一条应是提交顺序里的质量条款，得到 %q seq=%d",
			c.Clauses[0].Title, c.Clauses[0].Seq)
	}
	if c.Clauses[1].Title != pay.Title || c.Clauses[1].Seq != 2 {
		t.Errorf("第二条应是付款条款，得到 %q seq=%d", c.Clauses[1].Title, c.Clauses[1].Seq)
	}
	if c.Clauses[0].ClauseID == nil || *c.Clauses[0].ClauseID != qual.ID {
		t.Error("快照要记下来源模板 id，事后能回答「这条从哪来」")
	}

	// 改模板正文：已生成的合同不能跟着变。
	before := c.Clauses[1].Body
	if _, err := fx.clauseSvc.Update(ctx, pay.ID, dto.ClauseInput{
		Code: "PAY-30", Title: "付款条件", Category: "付款",
		Body: "改成 50% 订金。",
	}); err != nil {
		t.Fatalf("改模板失败: %v", err)
	}
	again, err := fx.svc.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("重新读合同失败: %v", err)
	}
	if again.Clauses[1].Body != before {
		t.Errorf("改模板不该影响已生成的合同：\n改前 %q\n改后 %q", before, again.Clauses[1].Body)
	}

	t.Run("停用的模板不能再用来生成合同", func(t *testing.T) {
		off := int8(model.EnableStatusOff)
		if _, err := fx.clauseSvc.Update(ctx, pay.ID, dto.ClauseInput{
			Code: "PAY-30", Title: "付款条件", Body: "x", Status: &off,
		}); err != nil {
			t.Fatalf("停用模板失败: %v", err)
		}

		q2 := fx.confirmedQuotation(t)
		_, err := fx.svc.Generate(ctx, dto.ContractGenerateInput{
			Type:       int8(model.ContractTypePI),
			SourceType: int8(model.ContractSourceQuotation),
			SourceID:   q2.ID,
			SignDate:   "2026-10-28",
			Terms: []dto.ContractTermInput{
				{Name: "全额", Amount: money.MustParse("1000")},
			},
			ClauseIDs: []uint{pay.ID},
		})
		wantCode(t, err, errs.CodeInvalidParam, "已停用的模板")
	})

	t.Run("选中不存在的模板要报出是哪一条", func(t *testing.T) {
		q3 := fx.confirmedQuotation(t)
		_, err := fx.svc.Generate(ctx, dto.ContractGenerateInput{
			Type:       int8(model.ContractTypePI),
			SourceType: int8(model.ContractSourceQuotation),
			SourceID:   q3.ID,
			SignDate:   "2026-10-28",
			Terms: []dto.ContractTermInput{
				{Name: "全额", Amount: money.MustParse("1000")},
			},
			ClauseIDs: []uint{9999},
		})
		wantCode(t, err, errs.CodeInvalidParam, "模板不存在")
	})
}
