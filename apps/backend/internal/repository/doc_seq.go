package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

// docSeqMaxAttempts 是发号的冲突重试次数。
//
// 冲突只可能来自"号段还不存在时的并发插入"，正常情况下第一次就成功。
// 给 5 次是为了在外键冲突叠加少量 sqlite 写锁重试时仍能发出去，而不是把
// 一次瞬时争用暴露成 500 —— 发号失败意味着单据存不下来。
const docSeqMaxAttempts = 5

// DocSeqRepository 负责发号。
//
// 单独抽出来而不是塞进各单据的仓储：单号的唯一性要在"单据表"之外保证，
// 否则只能靠单据表的 uniqueIndex 兜底，而那已经是 500 而不是重试了。
type DocSeqRepository interface {
	// Next 取下一个序号。同一 (docType, period) 的并发调用必须拿到互不相同的值。
	Next(ctx context.Context, docType model.DocType, period string) (uint, error)
}

type docSeqRepo struct {
	db *gorm.DB
}

func NewDocSeqRepository(db *gorm.DB) DocSeqRepository {
	return &docSeqRepo{db: db}
}

func (r *docSeqRepo) Next(ctx context.Context, docType model.DocType, period string) (uint, error) {
	var lastErr error
	for attempt := 0; attempt < docSeqMaxAttempts; attempt++ {
		seq, err := r.nextOnce(ctx, docType, period)
		if err == nil {
			return seq, nil
		}
		lastErr = err
	}
	return 0, fmt.Errorf("分配单号 %s-%s（已重试 %d 次）: %w",
		docType, period, docSeqMaxAttempts, lastErr)
}

// nextOnce 在一个事务里"自增并读回"。
//
// 为什么不是"查最大号 +1"：两个并发请求会查到同一个值，其中一个必然撞单号上的
// 唯一索引，而那时错误已经是 500，用户只会看到"保存失败"。
//
// 为什么读回是安全的：UPDATE 在事务里已经锁住这一行（sqlite 是库级写锁，
// pg/mysql 是行锁），别的请求的 UPDATE 只能等本事务提交，它读回的一定是
// 自己这次自增的结果。若不锁行而是"先 UPDATE 再单独 SELECT"，
// 两个请求可能读到同一个更大的值，反而造出重号。
func (r *docSeqRepo) nextOnce(ctx context.Context, docType model.DocType, period string) (uint, error) {
	var seq uint

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.DocSeq{}).
			Where("doc_type = ? AND period = ?", docType, period).
			Update("seq", gorm.Expr("seq + 1"))
		if res.Error != nil {
			return res.Error
		}

		if res.RowsAffected == 0 {
			// 号段还不存在：插入第一条。并发插入由联合唯一索引拦下，
			// 失败的那一方在外层重试，此时会走上面的自增分支。
			return tx.Create(&model.DocSeq{DocType: docType, Period: period, Seq: 1}).Error
		}

		var row model.DocSeq
		if err := tx.Where("doc_type = ? AND period = ?", docType, period).First(&row).Error; err != nil {
			return err
		}
		seq = row.Seq
		return nil
	})
	if err != nil {
		return 0, err
	}
	if seq == 0 {
		// 插入分支：本次发的是 0001。
		seq = 1
	}
	return seq, nil
}
