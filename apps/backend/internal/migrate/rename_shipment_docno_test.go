package migrate

import (
	"log/slog"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
)

func TestRenameShipmentDocNos(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	shipDate := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

	t.Run("旧号按原序号重编并留对照", func(t *testing.T) {
		db := newTestDB(t)
		if err := db.AutoMigrate(&model.Shipment{}, &model.DocSeq{}, &model.DocNoRename{}); err != nil {
			t.Fatalf("建表: %v", err)
		}

		// 新格式的 001 已经被占：重编时必须绕开它，否则撞 doc_no 唯一索引。
		if err := db.Create(&model.Shipment{
			DocNo: "DN-2610-001", Status: model.DocumentStatusDraft, ShipDate: shipDate,
		}).Error; err != nil {
			t.Fatalf("建新格式送货单: %v", err)
		}
		legacy := []model.Shipment{
			{DocNo: "SH-202610-0001", Status: model.DocumentStatusDraft, ShipDate: shipDate},
			{DocNo: "SH-202610-0002", Status: model.DocumentStatusDraft, ShipDate: shipDate},
			{DocNo: "SH-202611-0001", Status: model.DocumentStatusDraft, ShipDate: shipDate},
		}
		for i := range legacy {
			if err := db.Create(&legacy[i]).Error; err != nil {
				t.Fatalf("建旧格式送货单: %v", err)
			}
		}

		if err := RenameShipmentDocNos(db, log); err != nil {
			t.Fatalf("重编失败: %v", err)
		}

		wantByID := map[uint]string{
			legacy[0].ID: "DN-2610-002", // 001 被占用，顺延
			legacy[1].ID: "DN-2610-003",
			legacy[2].ID: "DN-2611-001", // 跨月各算各的
		}
		for id, want := range wantByID {
			var got model.Shipment
			if err := db.First(&got, id).Error; err != nil {
				t.Fatalf("查送货单 %d: %v", id, err)
			}
			if got.DocNo != want {
				t.Errorf("送货单 %d 重编为 %q，期望 %q", id, got.DocNo, want)
			}
		}

		// 新旧对照表：客户手里那张 SH 号必须能查回来。
		var renames []model.DocNoRename
		if err := db.Order("old_no").Find(&renames).Error; err != nil {
			t.Fatalf("查对照表: %v", err)
		}
		if len(renames) != 3 {
			t.Fatalf("对照记录 = %d 条，期望 3", len(renames))
		}
		if renames[0].OldNo != "SH-202610-0001" || renames[0].NewNo != "DN-2610-002" {
			t.Errorf("对照首条 = %s → %s，期望 SH-202610-0001 → DN-2610-002",
				renames[0].OldNo, renames[0].NewNo)
		}

		// 号段要追上重编后的最大序号，否则下一条新单会撞已占用的号。
		for period, want := range map[string]uint{"202610": 3, "202611": 1} {
			var seq model.DocSeq
			if err := db.Where("doc_type = ? AND period = ?", model.DocTypeShipment, period).
				First(&seq).Error; err != nil {
				t.Fatalf("查号段 %s: %v", period, err)
			}
			if seq.Seq != want {
				t.Errorf("号段 %s = %d，期望 %d", period, seq.Seq, want)
			}
		}

		// 幂等：再跑一次不改任何东西。
		if err := RenameShipmentDocNos(db, log); err != nil {
			t.Fatalf("重复执行失败: %v", err)
		}
		var count int64
		if err := db.Model(&model.DocNoRename{}).Count(&count).Error; err != nil {
			t.Fatalf("统计对照表: %v", err)
		}
		if count != 3 {
			t.Errorf("重复执行后对照记录 = %d 条，期望仍是 3", count)
		}
	})

	t.Run("结构不对的旧号整批停下", func(t *testing.T) {
		db := newTestDB(t)
		if err := db.AutoMigrate(&model.Shipment{}, &model.DocSeq{}, &model.DocNoRename{}); err != nil {
			t.Fatalf("建表: %v", err)
		}
		// 前缀是 SH- 但年月不是 6 位：手工改过号，不能猜着重编。
		if err := db.Create(&model.Shipment{
			DocNo: "SH-2610-1", Status: model.DocumentStatusDraft, ShipDate: shipDate,
		}).Error; err != nil {
			t.Fatalf("建异常送货单: %v", err)
		}

		if err := RenameShipmentDocNos(db, log); err == nil {
			t.Fatal("结构不符的旧号应该报错而不是猜一个")
		}
	})
}
