package bootstrap

import (
	"context"
	"fmt"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// seedCRM 写入联系人、线索、商机与跟进记录。
//
// 跟进记录里刻意放两条"下次跟进日早于今天"的：待办页的「待跟进」判定的是
// **早于今天（当天不算）**，没有逾期数据时那一块永远是空的，看不出判定对不对。
func (a *App) seedCRM(ctx context.Context, d seedDeps, m *demoMaster) error {
	primary := true
	notPrimary := false

	contacts := []struct {
		partnerCode, name, title, phone, email, language, channel string
		primary                                                   *bool
	}{
		{"CUS001", "Carlos Mendoza", "采购经理", "+593 9 0000 0001", "carlos@elsol.com.ec", "es", "WhatsApp", &primary},
		{"CUS001", "Maria Paredes", "财务", "+593 9 0000 0003", "maria@elsol.com.ec", "es", "Email", &notPrimary},
		{"CUS005", "Luis Ramírez", "品类采购", "+52 55 0000 0001", "luis@hdmexico.com", "en", "Email", &primary},
		{"CUS004", "Ana Torres", "总经理", "+507 6000 0001", "ana@central.com.pa", "es", "WhatsApp", &primary},
		{"SUP001", "陈先生", "销售", "13800000001", "chen@huaxin-hardware.cn", "zh", "微信", &primary},
	}
	for _, c := range contacts {
		partner, ok := m.customers[c.partnerCode]
		if !ok {
			partner = m.suppliers[c.partnerCode]
		}
		if partner == nil {
			continue
		}
		in := dto.ContactInput{
			PartnerID: partner.ID,
			Name:      c.name,
			Title:     c.title,
			Phone:     c.phone,
			Email:     c.email,
			Language:  c.language,
			Channel:   c.channel,
			IsPrimary: c.primary,
		}
		if _, err := d.Contacts.Create(ctx, in); err != nil {
			return fmt.Errorf("写入联系人 %s: %w", c.name, err)
		}
	}

	leads := []struct {
		company, contact, phone, email, country, source, demand string
		status                                                  model.LeadStatus
	}{
		{"Ferreteria Los Andes", "Diego Salas", "+56 9 0000 0001", "diego@losandes.cl", "Chile", string(model.LeadSourceAlibaba), "询不锈钢合页 4 寸，月需求约 2 万件", model.LeadStatusNew},
		{"Casa del Herraje", "Sofía Ruiz", "+52 33 0000 0002", "sofia@casadelherraje.mx", "Mexico", string(model.LeadSourceExhibition), "广交会见过的客户，想看门锁与拉手全套", model.LeadStatusFollowing},
		{"Buildmart Panama", "Jorge Lee", "+507 6000 0009", "jorge@buildmart.pa", "Panama", string(model.LeadSourceReferral), "El Sol 介绍，问五金件拼柜报价", model.LeadStatusFollowing},
		{"Cerrajería Bogotá", "Andrés Gómez", "+57 1 000 0003", "andres@cerrajeriabogota.co", "Colombia", string(model.LeadSourceGoogle), "只做锁具，量小", model.LeadStatusDropped},
	}
	for _, l := range leads {
		status := int8(l.status)
		in := dto.LeadInput{
			Company:     l.company,
			ContactName: l.contact,
			Phone:       l.phone,
			Email:       l.email,
			Country:     l.country,
			Source:      l.source,
			Demand:      l.demand,
			Status:      &status,
		}
		if _, err := d.Leads.Create(ctx, in); err != nil {
			return fmt.Errorf("写入线索 %s: %w", l.company, err)
		}
	}

	opportunities := []struct {
		partnerCode, subject string
		stage                model.OpportunityStage
		amount               string
		closeIn              int
	}{
		{"CUS004", "巴拿马 Central 2026 年度五金采购", model.OpportunityStageQuoted, "86000", 25},
		{"CUS001", "El Sol 门锁系列换供应商", model.OpportunityStageNegotiating, "52000", 12},
		{"CUS005", "Home Depot 墨西哥 Q1 补货", model.OpportunityStageWon, "148000", -6},
		{"BOTH001", "华翔包装辅料年度框架", model.OpportunityStageQualify, "18000", 40},
	}
	for _, o := range opportunities {
		partner := m.customers[o.partnerCode]
		if partner == nil {
			continue
		}
		in := dto.OpportunityInput{
			PartnerID:         partner.ID,
			Subject:           o.subject,
			Stage:             int8(o.stage),
			Amount:            money.MustParse(o.amount),
			CurrencyCode:      "USD",
			ExpectedCloseDate: day(o.closeIn),
		}
		if _, err := d.Opportunities.Create(ctx, in); err != nil {
			return fmt.Errorf("写入商机 %s: %w", o.subject, err)
		}
	}

	activities := []struct {
		partnerCode, kind, subject, content string
		occurredIn, nextIn                  int
	}{
		{"CUS001", string(model.ActivityTypeVisit), "Guayaquil 拜访：确认 Q1 订单结构", "客户希望把门锁比例提到 30%，交期压到 30 天。", -28, -3},
		{"CUS004", string(model.ActivityTypeEmail), "发送 2026 年报价单与样品清单", "已附上 FOB 价与新样品清单，等待客户内部评审。", -12, 4},
		{"CUS005", string(model.ActivityTypeCall), "电话确认 LC 开证行", "对方财务确认由 Banorte 开证，需要我们先出 PI。", -6, 1},
		{"CUS001", string(model.ActivityTypeQuote), "门锁系列重新报价", "按 5000 套阶梯价给出 17.6 美元/套。", -20, -8},
		{"BOTH001", string(model.ActivityTypeOther), "客供物料对账", "包装纸箱由华翔供货，本月对账差异 240 元。", -9, 15},
		{"CUS006", string(model.ActivityTypeEmail), "账期说明与停用通知", "客户要求 OA 90 天，超出政策，暂缓合作。", -40, 0},
	}
	for _, act := range activities {
		partner := m.customers[act.partnerCode]
		if partner == nil {
			continue
		}
		in := dto.ActivityInput{
			PartnerID:    partner.ID,
			Type:         act.kind,
			Subject:      act.subject,
			Content:      act.content,
			OccurredAt:   day(act.occurredIn),
			NextFollowAt: nextDay(act.nextIn),
		}
		if _, err := d.Activities.Create(ctx, in); err != nil {
			return fmt.Errorf("写入跟进记录 %s: %w", act.subject, err)
		}
	}

	a.Log.Info("已写入演示 CRM 数据",
		"contacts", len(contacts), "leads", len(leads),
		"opportunities", len(opportunities), "activities", len(activities))
	return nil
}

// nextDay 把"零偏移"当作不安排下次跟进（返回空串），而不是写成今天。
//
// 写今天会让待办里的「待跟进」把它算成已到期（判定是早于今天），
// 于是一条本来"已经跟完了"的记录会永远挂在待办列表里。
func nextDay(offset int) string {
	if offset == 0 {
		return ""
	}
	return day(offset)
}
