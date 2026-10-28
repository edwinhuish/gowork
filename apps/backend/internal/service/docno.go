package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// DocNoService 是全站唯一的单号来源。
//
// 让每张单据各写一套"前缀 + 日期 + 流水"的后果不是重复代码，而是格式漂移：
// 报价单用 YYYYMMDD、订单用 YYMM，两个月后没有人能按单号排序。
type DocNoService interface {
	// Next 取当前年月的下一个单号，如 PO-202610-0001。
	Next(ctx context.Context, docType model.DocType) (string, error)
	// NextDeal 为一笔新生意发案号，并给出该案号下这张单的单号。
	NextDeal(ctx context.Context, key DealKey, docType model.DocType) (DealNo, error)
}

// DealNo 是一次案号发放的结果。
type DealNo struct {
	// BaseNo 是案号，如 DZ2603：报价与它转出的订单共用同一个。
	BaseNo string
	// DocNo 是这张单的单号，如 DZ2603-QT01。
	DocNo string
	// Seq 是案号内序号（01 → 1），转单时订单沿用它。
	Seq uint
}

// DealKey 是案号号段的键：客户 + 年。
type DealKey struct {
	PartnerID   uint
	PartnerCode string
	Year        int
}

// dealSeqMax 是案号内序号的上限（两位数字）。
//
// 用尽就报错，不把单号撑成三位：案号是印在纸上、写在邮件里的分段格式，
// 多一位会让所有按列宽核对单号的表格错位，也会让「编码 + 年 + 序号」的切分变歧义。
const dealSeqMax = 99

type docNoService struct {
	seq   repository.DocSeqRepository
	clock func() time.Time
	log   *slog.Logger
}

type DocNoOption func(*docNoService)

func WithDocNoLogger(l *slog.Logger) DocNoOption {
	return func(s *docNoService) { s.log = l }
}

// WithDocNoClock 注入时钟，测试用来跨月验证号段重置，不需要真的等到下个月。
func WithDocNoClock(now func() time.Time) DocNoOption {
	return func(s *docNoService) { s.clock = now }
}

func NewDocNoService(seq repository.DocSeqRepository, opts ...DocNoOption) DocNoService {
	s := &docNoService{seq: seq, clock: time.Now, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *docNoService) Next(ctx context.Context, docType model.DocType) (string, error) {
	if !docType.IsValid() {
		// 不校验的话，拼错的类型会安静地开一条新号段，
		// 表现是"单号从 0001 重新开始"而不报任何错。
		return "", errs.BadRequest("未知的单据类型：" + string(docType))
	}
	// 报价与销售订单不走月号段：它们的号由案号派生（DZ2603-QT01）。
	// 留一条后路让它们也能从月号段发号，同一张表里就会长出两种格式的单号。
	switch docType {
	case model.DocTypeQuotation, model.DocTypeSalesOrder:
		return "", errs.BadRequest("报价单与销售订单按案号发号，请用 NextDeal")
	}

	// 年月取本地日历月：东八区 10 月 31 日 23:00 的单据不应拿到 9 月的号段。
	period := model.DocNoPeriod(s.clock())
	seq, err := s.seq.Next(ctx, docType, period)
	if err != nil {
		return "", err
	}
	// 序号位数是格式的一部分（PO-2610-001 是三位），用尽必须停下报错。
	if max := model.MaxSeq(docType); seq > max {
		return "", errs.ErrConflict.WithMsg(fmt.Sprintf(
			"%s 在 %s 的号段已用尽（%d 张），请等下个月或改用其它单据类型",
			docType, period, max))
	}
	return model.FormatDocNo(docType, period, seq), nil
}

func (s *docNoService) NextDeal(
	ctx context.Context, key DealKey, docType model.DocType,
) (DealNo, error) {
	switch docType {
	case model.DocTypeQuotation, model.DocTypeSalesOrder:
	default:
		return DealNo{}, errs.BadRequest("案号只用于报价单与销售订单：" + string(docType))
	}
	if key.PartnerID == 0 || strings.TrimSpace(key.PartnerCode) == "" {
		return DealNo{}, errs.BadRequest("发案号需要客户主体与编码")
	}
	if key.Year <= 0 {
		return DealNo{}, errs.BadRequest("发案号需要年份")
	}

	seq, err := s.seq.Next(ctx, model.DocTypeDeal, dealPeriod(key))
	if err != nil {
		return DealNo{}, err
	}
	if seq > dealSeqMax {
		return DealNo{}, errs.ErrConflict.WithMsg(fmt.Sprintf(
			"客户 %s 在 %d 年的案号已用尽（%d 笔），次年从 01 重新开始",
			key.PartnerCode, key.Year, dealSeqMax))
	}

	baseNo := model.DealBaseNo(key.PartnerCode, key.Year, seq)
	return DealNo{
		BaseNo: baseNo,
		DocNo:  model.FormatDealDocNo(baseNo, docType, seq),
		Seq:    seq,
	}, nil
}

// dealPeriod 是案号在号段表里的键。
//
// 用 PartnerID 而不是编码：编码可以改（合并主体、纠拼写），改一次就把号段
// 劈成两半——同一个客户次年从 01 重来的规则也就不成立了。
// 年存 4 位：2100 年不该和 2026 年落到同一行。
func dealPeriod(key DealKey) string {
	return fmt.Sprintf("deal:%d:%04d", key.PartnerID, key.Year)
}
