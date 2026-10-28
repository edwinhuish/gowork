package service

import (
	"context"
	"log/slog"
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
	// Next 取当前年月的下一个单号，如 SO-202610-0001。
	Next(ctx context.Context, docType model.DocType) (string, error)
}

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

	// 年月取本地日历月：东八区 10 月 31 日 23:00 的单据不应拿到 9 月的号段。
	period := model.DocNoPeriod(s.clock())
	seq, err := s.seq.Next(ctx, docType, period)
	if err != nil {
		return "", err
	}
	return model.FormatDocNo(docType, period, seq), nil
}
