package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// ContractService 管形式发票与销售合同。
//
// 它不复制货物明细，只做三件事：从来源单据抄一份快照（客户、币种、汇率、总额）、
// 校验付款分期合计等于总额、管草稿到确认这一步。金额一律不由前端提交：
// 合同是对客户的承诺，任何一项可以手填，它就和订单对不上了。
type ContractService interface {
	// Generate 从报价单或销售订单生成一份合同草稿。
	// 没有"空白新建"：合同必须有来源单据，否则金额从哪来就答不上。
	Generate(ctx context.Context, in dto.ContractGenerateInput) (*model.Contract, error)
	// InvoiceTerm 给合同某一期开票（T-61）：生成一张应收并回写该期。
	InvoiceTerm(ctx context.Context, contractID, termID uint) (*model.Receivable, error)
	Update(ctx context.Context, id uint, in dto.ContractUpdateInput) (*model.Contract, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Contract, error)
	List(ctx context.Context, q dto.ContractQuery) (*dto.PageResult[model.Contract], error)
	UpdateStatus(ctx context.Context, id uint, in dto.ContractStatusInput) (*model.Contract, error)
}

type contractService struct {
	contracts  repository.ContractRepository
	quotations repository.QuotationRepository
	orders     repository.SalesOrderRepository
	currencies CurrencyService
	docNos     DocNoService
	// partners 只用来判数据范围（T-32）：合同归属跟着客户走，
	// 而合同的 partner_id 只在库里，服务手上没有别的办法知道这单是谁的。
	partners repository.PartnerRepository
	// receivables 是惰性取值的应收创建入口（T-61）。
	//
	// 用函数而不是直接持有接口：应收服务的构造不依赖合同，而合同的构造依赖应收，
	// 装配顺序由 bootstrap 决定。传一个取值函数，两边都不必为了顺序让步。
	receivables func() ReceivableCreator
	// clauseRepo 用来把条款模板抄成快照（T-27）。合同服务不依赖条款服务的
	// 业务逻辑，只读模板正文，所以直接拿仓储。
	clauseRepo repository.ClauseRepository
	// receivableRepo 只用于详情页读「这期收了多少钱」（T-61）：
	// 写入走上面那个惰性函数，两边各管一头，互不影响装配顺序。
	receivableRepo repository.ReceivableRepository
	clock          func() time.Time
	log            *slog.Logger
}

// ReceivableCreator 是合同分期开票要用的那一件事：给我开一张应收。
type ReceivableCreator interface {
	Create(ctx context.Context, in dto.ReceivableInput) (*model.Receivable, error)
}

// WithContractReceivables 注入应收创建入口（T-61）。
func WithContractReceivables(f func() ReceivableCreator) ContractOption {
	return func(s *contractService) { s.receivables = f }
}

type ContractOption func(*contractService)

func WithContractLogger(l *slog.Logger) ContractOption {
	return func(s *contractService) { s.log = l }
}

// WithContractClock 注入时钟：单号年月、签订日默认值、来源是否过期都要可断言。
func WithContractClock(now func() time.Time) ContractOption {
	return func(s *contractService) { s.clock = now }
}

func NewContractService(
	contracts repository.ContractRepository,
	quotations repository.QuotationRepository,
	orders repository.SalesOrderRepository,
	currencies CurrencyService,
	docNos DocNoService,
	partners repository.PartnerRepository,
	clauseRepo repository.ClauseRepository,
	receivableRepo repository.ReceivableRepository,
	opts ...ContractOption,
) ContractService {
	s := &contractService{
		contracts:      contracts,
		quotations:     quotations,
		orders:         orders,
		currencies:     currencies,
		docNos:         docNos,
		partners:       partners,
		clauseRepo:     clauseRepo,
		receivableRepo: receivableRepo,
		clock:          time.Now,
		log:            slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *contractService) Generate(
	ctx context.Context, in dto.ContractGenerateInput,
) (*model.Contract, error) {
	ct := model.ContractType(in.Type)
	if !ct.IsValid() {
		return nil, errs.BadRequest("type 只能是 10（形式发票）或 20（销售合同）")
	}
	st := model.ContractSourceType(in.SourceType)
	if !st.IsValid() {
		return nil, errs.BadRequest("source_type 只能是 10（报价单）或 20（销售订单）")
	}

	src, err := s.loadSource(ctx, st, in.SourceID)
	if err != nil {
		return nil, err
	}

	// 同一张来源单据对同一种合同只能生成一次。查重带 type：
	// 先出 PI 收订金、成交后再出销售合同是正常流程，不该被彼此的查重挡住。
	dup, err := s.contracts.FindBySource(ctx, st, in.SourceID, ct)
	if err != nil {
		return nil, err
	}
	if dup != nil {
		return nil, errs.ErrConflict.WithMsg(fmt.Sprintf(
			"%s %s 已生成过%s %s", st.Label(), src.docNo, ct.Label(), dup.DocNo))
	}

	signDate, err := parseDateOrDefault(in.SignDate, s.clock())
	if err != nil {
		return nil, err
	}
	deliveryDate, err := resolveContractDelivery(in.DeliveryDate, src.deliveryDate, signDate)
	if err != nil {
		return nil, err
	}
	terms, err := s.buildTerms(ctx, in.Terms, src)
	if err != nil {
		return nil, err
	}
	// 条款在生成时**抄成快照**：之后模板怎么改，这份合同上写的还是当时那几句话。
	clauses, err := s.buildClauses(ctx, in.ClauseIDs)
	if err != nil {
		return nil, err
	}

	// 取号排在最后：前面任何一步失败都不该消耗掉一个单号。
	docNo, err := s.docNos.Next(ctx, contractDocType(ct))
	if err != nil {
		return nil, err
	}

	c := &model.Contract{
		DocNo:           docNo,
		Type:            ct,
		PartnerID:       src.partnerID,
		SourceType:      st,
		SourceID:        src.id,
		SourceDocNo:     src.docNo,
		Status:          model.DocumentStatusDraft,
		SignDate:        signDate,
		DeliveryDate:    deliveryDate,
		CurrencyCode:    src.currencyCode,
		ExchangeRate:    src.exchangeRate,
		AmountTotal:     src.amountTotal,
		AmountBaseTotal: src.amountBase,
		Remark:          in.Remark,
	}
	if err := s.contracts.CreateWithTerms(ctx, c, terms, clauses); err != nil {
		return nil, err
	}

	s.log.Info("生成合同", "id", c.ID, "doc_no", c.DocNo, "type", ct.Label(),
		"source", src.docNo, "amount_total", c.AmountTotal, "terms", len(terms))
	return s.contracts.FindByID(ctx, c.ID)
}

func (s *contractService) Update(
	ctx context.Context, id uint, in dto.ContractUpdateInput,
) (*model.Contract, error) {
	existing, err := s.contracts.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !existing.IsEditable() {
		return nil, errs.ErrConflict.WithMsg(
			"只有草稿可以修改；已确认的合同是对客户的承诺，改动应作废重开")
	}

	// 签订日留空保持原值：它已经印在单号年月和发出去的 PDF 上了，
	// 不因为这次没填就变成今天。
	signDate := existing.SignDate
	if strings.TrimSpace(in.SignDate) != "" {
		signDate, err = parseDateOnly(in.SignDate)
		if err != nil {
			return nil, err
		}
	}

	fields := map[string]any{
		"sign_date":     signDate,
		"delivery_date": nil,
		"remark":        in.Remark,
	}
	// 交期留空表示清空：它与签订日不同，本来就可空，谈掉了就该消失。
	if strings.TrimSpace(in.DeliveryDate) != "" {
		d, err := parseDateOnly(in.DeliveryDate)
		if err != nil {
			return nil, err
		}
		if d.Before(signDate) {
			return nil, errs.BadRequest("交期不能早于签订日")
		}
		fields["delivery_date"] = d
	}

	terms, err := s.buildTerms(ctx, in.Terms, contractSourceOf(existing))
	if err != nil {
		return nil, err
	}
	// 条款整体替换（同样是快照）：草稿阶段改条款是正常的，确认之后就没得改了。
	clauses, err := s.buildClauses(ctx, in.ClauseIDs)
	if err != nil {
		return nil, err
	}
	if in.UpdatedAt != nil {
		fields[repository.FieldUpdatedAt] = *in.UpdatedAt
	}
	if err := s.contracts.UpdateWithTerms(ctx, id, fields, terms, clauses); err != nil {
		return nil, err
	}

	s.log.Info("修改合同", "id", id, "doc_no", existing.DocNo, "terms", len(terms))
	return s.contracts.FindByID(ctx, id)
}

func (s *contractService) Delete(ctx context.Context, id uint) error {
	existing, err := s.contracts.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !existing.IsEditable() {
		// 已确认的合同留在库里供追溯；不要了走"取消"，而不是抹掉。
		return errs.ErrConflict.WithMsg("只有草稿可以删除；已确认的合同请改为已取消")
	}
	return s.contracts.Delete(ctx, id)
}

func (s *contractService) Get(ctx context.Context, id uint) (*model.Contract, error) {
	m, err := s.contracts.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 越权 403 而不是 404：合同确实存在，只是不归你看。
	if err := ensurePartnerVisible(ctx, s.partners, m.PartnerID); err != nil {
		return nil, err
	}
	if err := s.fillTermSettlements(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// fillTermSettlements 给每一期补上「已收 / 已冲减 / 未结清」（T-61）。
//
// 这三个数不落库：它们是应收单的现状，落了就要跟着每一次核销同步一次，
// 而真相只有一个——应收单上的金额与核销表。没开票的那一期按"整期未收"算，
// 那正是业务要看的东西：这一期该收多少、还差多少。
func (s *contractService) fillTermSettlements(
	ctx context.Context, c *model.Contract,
) error {
	if len(c.Terms) == 0 {
		return nil
	}
	if s.receivableRepo == nil {
		return nil
	}

	ids := make([]uint, 0, len(c.Terms))
	for _, t := range c.Terms {
		if t.ReceivableID != nil {
			ids = append(ids, *t.ReceivableID)
		}
	}
	settlements := map[uint]repository.ReceivableSettlement{}
	if len(ids) > 0 {
		found, err := s.receivableRepo.Settlements(ctx, ids)
		if err != nil {
			return err
		}
		settlements = found
	}

	for i := range c.Terms {
		term := &c.Terms[i]
		term.OutstandingAmount = term.Amount
		if term.ReceivableID == nil {
			continue
		}
		info, ok := settlements[*term.ReceivableID]
		if !ok {
			// 应收单被硬删了（正常流程不会）：金额仍按整期未收算，
			// 不假装收过钱，也不因此让详情页打不开。
			continue
		}
		term.ReceivableDocNo = info.DocNo
		term.ReceivedAmount = info.Settled
		term.CreditedAmount = info.Credited
		term.OutstandingAmount = info.Amount.Sub(info.Settled).Sub(info.Credited)
	}
	return nil
}

func (s *contractService) List(
	ctx context.Context, q dto.ContractQuery,
) (*dto.PageResult[model.Contract], error) {
	q.PageQuery.Normalize()

	// 范围由身份决定：业务员只看自己（与未分配）客户的合同。
	filter := repository.ContractFilter{
		Keyword: q.Keyword, PartnerID: q.PartnerID, OwnerID: ownerScope(ctx),
	}
	if q.Type != nil {
		ct := model.ContractType(*q.Type)
		if !ct.IsValid() {
			return nil, errs.BadRequest("type 不合法")
		}
		filter.Type = &ct
	}
	if q.Status != nil {
		st := model.DocumentStatus(*q.Status)
		if !st.IsValid() {
			return nil, errs.BadRequest("status 不合法")
		}
		filter.Status = &st
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.contracts.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// UpdateStatus 只放行「确认」与「取消」。
//
// 合同没有备货中 / 已出货这些阶段：那些属于订单与出货批次。
// 不拦住的话 DocumentStatus 里的阶段迟早会被前端下拉框误用。
func (s *contractService) UpdateStatus(
	ctx context.Context, id uint, in dto.ContractStatusInput,
) (*model.Contract, error) {
	existing, err := s.contracts.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	next := model.DocumentStatus(in.Status)
	switch next {
	case model.DocumentStatusConfirmed, model.DocumentStatusCancelled:
	default:
		return nil, errs.BadRequest("合同只能转为「已确认」或「已取消」")
	}
	if err := checkDocumentTransit(existing.Status, next); err != nil {
		return nil, err
	}

	if next == model.DocumentStatusConfirmed {
		// 生成到确认之间可能过了几天：来源订单被取消而合同还停在草稿，
		// 确认下去就是对客户承诺一笔已经撤掉的生意。
		if err := s.ensureSourceAlive(ctx, existing); err != nil {
			return nil, err
		}
		// 确认就是承诺生效，此刻复核分期合计。保存时校验过一次，
		// 但金额可能从别的路径改过，"确认后才发现少一期"只能整单作废重开。
		dp, err := s.currencies.Decimals(ctx, existing.CurrencyCode)
		if err != nil {
			return nil, err
		}
		if err := checkTermSum(existing.Terms, existing.AmountTotal, dp); err != nil {
			return nil, err
		}
	}

	if err := s.contracts.Update(ctx, id, map[string]any{"status": next}); err != nil {
		return nil, err
	}
	s.log.Info("合同状态变更", "id", id, "doc_no", existing.DocNo,
		"from", existing.Status, "to", next)
	return s.contracts.FindByID(ctx, id)
}

// contractSource 是生成合同时从来源单据抄下来的快照。
//
// 收成独立结构而不是直接传 *model.Quotation / *model.SalesOrder：
// 两种单据的字段名一样但类型不同，收口之后"合同从哪张单抄哪几个字段"
// 只有一处答案，将来多一种来源也只需补一个分支。
type contractSource struct {
	id           uint
	docNo        string
	partnerID    uint
	currencyCode string
	exchangeRate money.Decimal
	amountTotal  money.Decimal
	amountBase   money.Decimal
	deliveryDate *time.Time
}

// loadSource 读来源单据并校验它此刻能不能出合同。
// buildClauses 把选中的模板抄成合同上的条款快照。
//
// 抄而不是引用：模板改了、甚至删了，已经签出去的合同都写着当时那几句话。
// 这也是"改动不影响已确认的合同"这条验收的结构性保证——不是靠校验，是靠没有引用。
//
// 顺序按提交顺序（不是按 id、也不是按模板的排序）：条款顺序是谈出来的，
// "以第 3 条为准"这类话得对得上纸面。
func (s *contractService) buildClauses(
	ctx context.Context, ids []uint,
) ([]model.ContractClause, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// 去重但保序：同一模板选两次是无意义的重复，顺序则是用户排的。
	seen := make(map[uint]struct{}, len(ids))
	ordered := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}

	templates, err := s.clauseRepo.FindByIDs(ctx, ordered)
	if err != nil {
		return nil, err
	}
	byID := make(map[uint]model.Clause, len(templates))
	for _, t := range templates {
		byID[t.ID] = t
	}

	out := make([]model.ContractClause, 0, len(ordered))
	for i, id := range ordered {
		t, ok := byID[id]
		if !ok {
			// 选了不存在的模板：报具体哪一条，而不是"参数错误"。
			return nil, errs.BadRequest(fmt.Sprintf("条款模板不存在或已删除：id=%d", id))
		}
		// 软删的拦截在上面这个分支：FindByIDs 走的是常规查询，
		// 已删除的模板查不出来，自然落到"不存在或已删除"。
		clauseID := t.ID
		out = append(out, model.ContractClause{
			ClauseID: &clauseID,
			Seq:      i + 1,
			Title:    t.Title,
			Body:     t.Body,
		})
	}
	return out, nil
}

func (s *contractService) loadSource(
	ctx context.Context, st model.ContractSourceType, id uint,
) (*contractSource, error) {
	switch st {
	case model.ContractSourceQuotation:
		q, err := s.quotations.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				return nil, errs.BadRequest("来源报价单不存在")
			}
			return nil, err
		}
		// 只有已确认的报价能出合同：草稿还没发给客户，价格随时可改。
		if q.Status != model.DocumentStatusConfirmed {
			return nil, errs.ErrConflict.WithMsg(
				"报价单当前状态不能生成合同：" + q.Status.Label() + "，请先确认")
		}
		// 过期的报价不再成立：据它出的合同，金额不是双方最后谈定的那个价。
		if q.IsExpired(s.clock()) {
			return nil, errs.ErrConflict.WithMsg(
				"报价单已过有效期（截止 " + q.ValidUntil.Format("2006-01-02") +
					"），请另存新版本后再生成合同")
		}
		return &contractSource{
			id:           q.ID,
			docNo:        q.DocNo,
			partnerID:    q.PartnerID,
			currencyCode: q.CurrencyCode,
			exchangeRate: q.ExchangeRate,
			amountTotal:  q.AmountTotal,
			amountBase:   q.AmountBaseTotal,
		}, nil

	case model.ContractSourceSalesOrder:
		o, err := s.orders.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				return nil, errs.BadRequest("来源销售订单不存在")
			}
			return nil, err
		}
		// 草稿订单的合计还能改，而合同是一份对外的承诺：先确认订单。
		if o.Status == model.DocumentStatusDraft {
			return nil, errs.ErrConflict.WithMsg("销售订单还是草稿，请先确认后再生成合同")
		}
		if o.Status == model.DocumentStatusCancelled {
			return nil, errs.ErrConflict.WithMsg("销售订单已取消，不能生成合同")
		}
		return &contractSource{
			id:           o.ID,
			docNo:        o.DocNo,
			partnerID:    o.PartnerID,
			currencyCode: o.CurrencyCode,
			exchangeRate: o.ExchangeRate,
			amountTotal:  o.AmountTotal,
			amountBase:   o.AmountBaseTotal,
			deliveryDate: o.DeliveryDate,
		}, nil
	}
	return nil, errs.BadRequest("未知的来源单据类型")
}

// ensureSourceAlive 复核来源单据没有被作废。
func (s *contractService) ensureSourceAlive(ctx context.Context, c *model.Contract) error {
	switch c.SourceType {
	case model.ContractSourceQuotation:
		q, err := s.quotations.FindByID(ctx, c.SourceID)
		if err != nil {
			return err
		}
		if q.Status == model.DocumentStatusCancelled {
			return errs.ErrConflict.WithMsg("来源报价单 " + q.DocNo + " 已取消，不能确认合同")
		}
	case model.ContractSourceSalesOrder:
		o, err := s.orders.FindByID(ctx, c.SourceID)
		if err != nil {
			return err
		}
		if o.Status == model.DocumentStatusCancelled {
			return errs.ErrConflict.WithMsg("来源销售订单 " + o.DocNo + " 已取消，不能确认合同")
		}
	}
	return nil
}

// buildTerms 校验分期并算出本币金额。
//
// 本币随行存一份而不是查询时再乘：分期报表要按本币汇总，
// 每次乘一遍既慢又容易和合同总额差一个末位（舍入位置不同）。
func (s *contractService) buildTerms(
	ctx context.Context, in []dto.ContractTermInput, src *contractSource,
) ([]model.ContractTerm, error) {
	if len(in) == 0 {
		return nil, errs.BadRequest("合同至少需要一个付款分期")
	}
	billDP, err := s.currencies.Decimals(ctx, src.currencyCode)
	if err != nil {
		return nil, err
	}
	baseCode, err := s.currencies.BaseCode(ctx)
	if err != nil {
		return nil, err
	}
	baseDP, err := s.currencies.Decimals(ctx, baseCode)
	if err != nil {
		return nil, err
	}

	terms := make([]model.ContractTerm, 0, len(in))
	for i, row := range in {
		name := strings.TrimSpace(row.Name)
		if name == "" {
			return nil, errs.BadRequest(fmt.Sprintf("第 %d 期缺少名称", i+1))
		}
		// 负金额的分期在业务上等于"退款"，合同里没有这个语义，要退就作废重开。
		if row.Amount.IsNegative() {
			return nil, errs.BadRequest(fmt.Sprintf("第 %d 期金额不能为负", i+1))
		}
		due, err := parseOptionalDate(row.DueDate)
		if err != nil {
			return nil, err
		}
		amount := row.Amount.Round(billDP)
		terms = append(terms, model.ContractTerm{
			Seq:          i + 1,
			Name:         name,
			Amount:       amount,
			AmountBase:   amount.Mul(src.exchangeRate).Round(baseDP),
			DueDate:      due,
			Trigger:      strings.TrimSpace(row.Trigger),
			ReceivableID: row.ReceivableID,
		})
	}

	// 分期合计等于合同总额是这份单据唯一能自证的验收条件。
	// 允许"差一点"就等于允许合同上写着 100 万、分期只收 90 万而没人发现。
	if err := checkTermSum(terms, src.amountTotal, billDP); err != nil {
		return nil, err
	}

	// 本币逐期舍入后，与合同本币总额之间会有末位差（来源单据本身也是
	// 逐行舍入再累加的）。差额并进最后一期：分期报表要能和合同总额对上，
	// 差一分钱没人查得动，而"合计少一分"会被当成漏了一期。
	// 差额超过"期数 × 一个最小单位"时不吃掉——那说明来源单据的本币总额
	// 与它锁定的汇率本身就不自洽，硬塞会把错误伪装成一次正常舍入。
	wantBase := src.amountBase.Round(baseDP)
	sumBase := money.Zero()
	for _, t := range terms {
		sumBase = sumBase.Add(t.AmountBase)
	}
	sumBase = sumBase.Round(baseDP)
	diff := wantBase.Sub(sumBase).Round(baseDP)
	if !diff.IsZero() {
		// 阈值是「期数 × 一个最小单位」，用定点算：10^-baseDP 就是 1e(6-baseDP) 个单位。
		tolerance := unitOf(baseDP).MulInt(int64(len(terms)))
		if diff.Abs().Cmp(tolerance) > 0 {
			return nil, errs.ErrInternal.WithMsg(fmt.Sprintf(
				"来源单据 %s 的本币合计 %s 与按合同汇率换算的 %s 不一致",
				src.docNo, wantBase, sumBase))
		}
		last := &terms[len(terms)-1]
		last.AmountBase = last.AmountBase.Add(diff).Round(baseDP)
	}
	return terms, nil
}

// contractSourceOf 把已有合同还原成来源快照，供编辑时复用同一套分期校验。
//
// 编辑不重新读来源单据：合同上的金额与汇率是生成那一刻锁定的，
// 来源单据后来改过（比如订单变更）也不该悄悄改掉这张合同。
func contractSourceOf(c *model.Contract) *contractSource {
	return &contractSource{
		id:           c.SourceID,
		docNo:        c.SourceDocNo,
		partnerID:    c.PartnerID,
		currencyCode: c.CurrencyCode,
		exchangeRate: c.ExchangeRate,
		amountTotal:  c.AmountTotal,
		amountBase:   c.AmountBaseTotal,
		deliveryDate: c.DeliveryDate,
	}
}

// checkTermSum 判定分期合计是否等于合同总额。
//
// 保存与确认共用这一个判据：一处放宽一处收紧，就会出现
// "保存得过、确认不过"，而报错里看不出该改哪一边。
// 判据取半个最小单位：两边都已在落库前按币种小数位舍入过，
// 剩下的差异只可能是浮点表示误差，不可能是真的少了一期。
func checkTermSum(terms []model.ContractTerm, total money.Decimal, decimalPlaces int) error {
	if len(terms) == 0 {
		return errs.BadRequest("合同至少需要一个付款分期")
	}
	sum := money.Zero()
	for _, t := range terms {
		sum = sum.Add(t.Amount)
	}
	// 两侧都已按币种小数位舍入，可以要求完全相等。float 时代留半个最小单位的
	// 容差是为了吸收二进制误差；定点之后还留容差，就等于允许合同少收一分钱。
	if sum.Round(decimalPlaces).Cmp(total.Round(decimalPlaces)) != 0 {
		return errs.BadRequest(fmt.Sprintf(
			"分期合计 %s 与合同总额 %s 不一致", sum, total))
	}
	return nil
}

// unitOf 返回该小数位下的一个最小单位（如 2 位小数的 0.01）。
func unitOf(dp int) money.Decimal {
	if dp >= money.Scale {
		return money.FromUnits(1)
	}
	return money.FromUnits(int64(math.Pow10(money.Scale - dp)))
}

// resolveContractDelivery 定下交期：入参优先，留空沿用来源单据的交期。
//
// 只有入参显式给了日期才与签订日比较：补签的合同其交期本来就可能早于签订日，
// 沿用来源值时把它判成非法，等于要求用户把真实交期改成假的。
func resolveContractDelivery(
	raw string, fallback *time.Time, signDate time.Time,
) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	d, err := parseDateOnly(raw)
	if err != nil {
		return nil, err
	}
	if d.Before(signDate) {
		return nil, errs.BadRequest("交期不能早于签订日")
	}
	return &d, nil
}

// contractDocType 定下这类合同用哪条号段：PI 与销售合同各自一条序列。
func contractDocType(t model.ContractType) model.DocType {
	if t == model.ContractTypeSales {
		return model.DocTypeSalesContract
	}
	return model.DocTypeProformaInvoice
}

// InvoiceTerm 给合同某一期开票（T-61）。
//
// 金额、币种、汇率全部取合同上的值：分期不是新的汇兑事件，它只是合同金额的
// 一部分，换汇率会让同一份合同的两期折出两个本币数。
func (s *contractService) InvoiceTerm(
	ctx context.Context, contractID, termID uint,
) (*model.Receivable, error) {
	c, err := s.contracts.FindByID(ctx, contractID)
	if err != nil {
		return nil, err
	}
	if c.Status == model.DocumentStatusDraft || c.Status == model.DocumentStatusCancelled {
		return nil, errs.ErrConflict.WithMsg("草稿或已取消的合同不能开票")
	}
	var term *model.ContractTerm
	for i := range c.Terms {
		if c.Terms[i].ID == termID {
			term = &c.Terms[i]
			break
		}
	}
	if term == nil {
		return nil, errs.BadRequest("这张合同里没有该分期")
	}
	if term.ReceivableID != nil {
		return nil, errs.ErrConflict.WithMsg("这一期已经开过票，不能重复开")
	}
	if s.receivables == nil {
		return nil, errs.New(errs.CodeInternal, "合同开票未接装配，请检查 bootstrap")
	}
	creator := s.receivables()
	if creator == nil {
		return nil, errs.New(errs.CodeInternal, "合同开票未接装配，请检查 bootstrap")
	}

	due := ""
	if term.DueDate != nil {
		due = model.DateOnly(*term.DueDate).Format("2006-01-02")
	}
	partnerID := c.PartnerID
	rate := c.ExchangeRate
	r, err := creator.Create(ctx, dto.ReceivableInput{
		SourceType:   int8(model.SettlementSourceContractTerm),
		SourceID:     term.ID,
		Amount:       term.Amount,
		DueDate:      due,
		PartnerID:    &partnerID,
		CurrencyCode: c.CurrencyCode,
		ExchangeRate: &rate,
		Remark:       "合同 " + c.DocNo + " 第 " + strconv.Itoa(term.Seq) + " 期：" + term.Name,
	})
	if err != nil {
		return nil, err
	}
	if err := s.contracts.UpdateTermReceivable(ctx, term.ID, r.ID); err != nil {
		return nil, err
	}
	s.log.Info("合同分期开票", "contract", c.DocNo, "term", term.Seq, "receivable", r.DocNo)
	return r, nil
}
