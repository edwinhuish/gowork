package migrate

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

const renameShipmentDocNoName = "rename_shipment_docno"

// RenameShipmentDocNos 把存量的 `SH-YYYYMM-NNNN` 重编为 `DN-YYMM-NNN`。
//
// 只改发号函数是不够的：已经发给客户的 `SH-` 号会留在纸质单与往来邮件里，
// 系统里却查不到，等于把历史单据弄丢。所以重编必须同时留下新旧对照表
// （`sys_doc_no_rename`），让"那个 SH 号现在是什么"有答案。
//
// 序号优先保留原值（SH-202610-0001 → DN-2610-001）：客户记得住尾号，
// 能不变就不变；只有当那个位置已经被新格式的单据占用时才顺延。
//
// 幂等靠 sys_migration 记录 + `LIKE 'SH-%'` 双重保证：迁移跑完就不再有旧前缀的行。
func RenameShipmentDocNos(db *gorm.DB, log *slog.Logger) error {
	if err := ensureRecordTable(db); err != nil {
		return err
	}
	done, err := applied(db, renameShipmentDocNoName)
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	var legacy []struct {
		ID    uint
		DocNo string
	}
	if err := db.Model(&model.Shipment{}).
		Select("id", "doc_no").
		Where("doc_no LIKE ?", "SH-%").
		Order("doc_no").
		Scan(&legacy).Error; err != nil {
		return fmt.Errorf("查询旧格式送货单: %w", err)
	}
	if len(legacy) == 0 {
		return markApplied(db, renameShipmentDocNoName)
	}

	used, err := shipmentSeqsInUse(db)
	if err != nil {
		return err
	}

	maxSeq := make(map[string]uint)
	renamed := make([]model.DocNoRename, 0, len(legacy))
	for _, row := range legacy {
		period, seq, ok := parseLegacyShipmentNo(row.DocNo)
		if !ok {
			// 前缀是 SH- 但结构不对：手工改过号，或者某次发号出过格式 bug。
			// 不猜，整批停下让人看一眼——重编一个猜错的号比重编失败更糟。
			return fmt.Errorf("送货单 %d 的单号 %q 不符合 SH-YYYYMM-NNNN，无法安全重编",
				row.ID, row.DocNo)
		}

		next := seq
		for {
			if _, taken := used[period][next]; !taken {
				break
			}
			next++
		}
		if next > model.MaxSeq(model.DocTypeShipment) {
			return fmt.Errorf("送货单 %s 所在的月份（%s）序号已用尽，无法重编", row.DocNo, period)
		}

		newNo := model.FormatDocNo(model.DocTypeShipment, period, next)
		if used[period] == nil {
			used[period] = make(map[uint]struct{})
		}
		used[period][next] = struct{}{}
		if next > maxSeq[period] {
			maxSeq[period] = next
		}

		renamed = append(renamed, model.DocNoRename{
			DocType:   model.DocTypeShipment,
			DocID:     row.ID,
			OldNo:     row.DocNo,
			NewNo:     newNo,
			RenamedAt: time.Now(),
		})
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		for _, r := range renamed {
			if err := tx.Model(&model.Shipment{}).
				Where("id = ?", r.DocID).
				Update("doc_no", r.NewNo).Error; err != nil {
				return fmt.Errorf("把 %s 重编为 %s: %w", r.OldNo, r.NewNo, err)
			}
			if err := tx.Create(&r).Error; err != nil {
				return fmt.Errorf("记录 %s → %s 的对照: %w", r.OldNo, r.NewNo, err)
			}
		}
		// 号段要追上重编后的最大序号，否则下一条新单会拿到已被占用的号。
		for period, seq := range maxSeq {
			if err := bumpShipmentSeq(tx, period, seq); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.Info("已把送货单号从 SH- 重编为 DN-", "count", len(renamed))
	return markApplied(db, renameShipmentDocNoName)
}

// usedSeqs 里存的是该月已占用的序号：包含已存在的新格式单号，以及本批刚分配的。
func shipmentSeqsInUse(db *gorm.DB) (map[string]map[uint]struct{}, error) {
	var nos []string
	if err := db.Model(&model.Shipment{}).
		Where("doc_no LIKE ?", "DN-%").
		Pluck("doc_no", &nos).Error; err != nil {
		return nil, fmt.Errorf("查询新格式送货单: %w", err)
	}

	used := make(map[string]map[uint]struct{}, len(nos))
	for _, no := range nos {
		period, seq, ok := parseShipmentNo(no)
		if !ok {
			continue
		}
		if used[period] == nil {
			used[period] = make(map[uint]struct{})
		}
		used[period][seq] = struct{}{}
	}
	return used, nil
}

// parseLegacyShipmentNo 拆 SH-202610-0001 → (202610, 1)。
func parseLegacyShipmentNo(docNo string) (string, uint, bool) {
	rest, ok := strings.CutPrefix(docNo, "SH-")
	if !ok {
		return "", 0, false
	}
	period, seqText, ok := strings.Cut(rest, "-")
	if !ok || len(period) != 6 {
		return "", 0, false
	}
	n, err := strconv.Atoi(seqText)
	if err != nil || n <= 0 {
		return "", 0, false
	}
	return period, uint(n), true
}

// parseShipmentNo 拆 DN-2610-001 → (202610, 1)。
//
// 单号里只印两位年，补世纪时取当前年份的前两位：2100 年运行时得到 2100，
// 而不是把 2100 与 2026 拼成同一串。
func parseShipmentNo(docNo string) (string, uint, bool) {
	rest, ok := strings.CutPrefix(docNo, "DN-")
	if !ok {
		return "", 0, false
	}
	ym, seqText, ok := strings.Cut(rest, "-")
	if !ok || len(ym) != 4 {
		return "", 0, false
	}
	n, err := strconv.Atoi(seqText)
	if err != nil || n <= 0 {
		return "", 0, false
	}
	// 单号里的年是两位（2610），期段存的是四位年（202610），补回世纪前缀。
	century := time.Now().Year() / 100 // 2026 → 20
	return fmt.Sprintf("%02d%s", century, ym), uint(n), true
}

// bumpShipmentSeq 把送货单号段推到 seq（只升不降：号段记的是"已发出的最大序号"）。
//
// 不用 GREATEST/MAX 标量函数：三个库的名字不一样，写成方言就违反了"换存储不改代码"。
// 这是一次性迁移，串行执行，没有并发窗口，先查再决定就够了。
func bumpShipmentSeq(tx *gorm.DB, period string, seq uint) error {
	var row model.DocSeq
	err := tx.Where("doc_type = ? AND period = ?", model.DocTypeShipment, period).
		First(&row).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return tx.Create(&model.DocSeq{
			DocType: model.DocTypeShipment, Period: period, Seq: seq,
		}).Error
	case err != nil:
		return fmt.Errorf("查询送货单号段 %s: %w", period, err)
	case row.Seq < seq:
		return tx.Model(&model.DocSeq{}).Where("id = ?", row.ID).Update("seq", seq).Error
	}
	return nil
}
