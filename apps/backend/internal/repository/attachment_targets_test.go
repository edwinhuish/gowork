package repository

import (
	"testing"

	"github.com/edwinhuish/gowork/internal/model"
)

// TestAttachTargetsMatchConstants 守住「附件能挂在哪」这张清单。
//
// attachTargets 是安全边界：不在表里的类型，上传一律 400。它有三种错法，
// 都不会在启动时报错，只在下游出事：
//   - 新增了 model.Attach* 常量却没加进表 → 前端能选，上传被拒；
//   - 表里留着一个改名或拼错的键 → 永远没人能传，白名单看起来却更宽；
//   - 表项指错模型（比如把 Receivable 写成 &model.Payable{}）→ Count 查错了表，
//     挂到任意 id 上都算「目标存在」，等于白名单失效。
//
// 前两条靠「常量集合与表键集合必须相等」，第三条靠「每个键映射的表名互不相同」。
func TestAttachTargetsMatchConstants(t *testing.T) {
	constants := []string{
		model.AttachPartner,
		model.AttachProduct,
		model.AttachQuotation,
		model.AttachContract,
		model.AttachSalesOrder,
		model.AttachPurchaseOrder,
		model.AttachInspection,
		model.AttachShipment,
		model.AttachCustomsDeclaration,
		model.AttachReceivable,
		model.AttachPayable,
		model.AttachPayment,
		model.AttachStockDocument,
	}

	for _, c := range constants {
		if _, ok := attachTargets[c]; !ok {
			t.Errorf("常量 %q 没有对应的挂载目标：前端能选，上传会被拒", c)
		}
	}
	if len(attachTargets) != len(constants) {
		t.Errorf("挂载目标 %d 项、常量 %d 个：表里可能有拼错或已改名的键",
			len(attachTargets), len(constants))
	}

	seen := make(map[string]string, len(attachTargets))
	for key, target := range attachTargets {
		table, ok := target.(interface{ TableName() string })
		if !ok {
			t.Errorf("挂载目标 %q 的模型没有 TableName()", key)
			continue
		}
		if other, dup := seen[table.TableName()]; dup {
			t.Errorf("%q 与 %q 指向同一张表 %s", key, other, table.TableName())
		}
		seen[table.TableName()] = key
	}
}
