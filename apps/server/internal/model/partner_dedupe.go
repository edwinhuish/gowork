package model

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 查重是"提示 + 人工确认"，不是校验：命中只说明像，合不合要看单据。
//
// 三条规则各自独立，命中任意一条就算"可能是同一家"：
//
//	name_country  名称（归一化后）与国家都相同
//	email_domain  邮箱同域，且不是免费邮箱
//	phone         电话数字尾 9 位相同
//
// 为什么不给主体表加归一化列（像产品编号那样落一份 code_norm）：
// 这里的比较在"人确认之后才合并"的场景里发生，主体表是几千行的量级，扫一遍比
// 维护三列归一值 + 回填迁移便宜；而且归一规则还在被现实调整（尾号取几位、
// 免费邮箱名单多长），落库意味着每改一次规则就要重刷一遍全表。
const (
	// PartnerDupNameCountry 名称归一化后相同，且国家也相同（都留空算相同）。
	PartnerDupNameCountry = "name_country"
	// PartnerDupEmailDomain 邮箱同域。免费邮箱不参与——两个客户都用 gmail
	// 不等于他们是一家。
	PartnerDupEmailDomain = "email_domain"
	// PartnerDupPhone 电话数字部分的尾 9 位相同，容忍国家码与前缀写法差异。
	PartnerDupPhone = "phone"
)

// PartnerProbe 是"待建 / 待改的一条主体"里参与查重的字段。
//
// 只有这四个：编码重复已经由唯一索引硬挡（409），地址与联系人姓名在
// 同一栋写字楼里可能完全一样（几家货代共用前台），拿来查重只会制造噪音。
type PartnerProbe struct {
	Name    string
	Country string
	Phone   string
	Email   string
	// ExcludeID 供编辑时排除自身。
	ExcludeID uint
}

// PartnerDuplicateCandidate 是一条命中：某个已有主体与 probe 像在哪里。
type PartnerDuplicateCandidate struct {
	Partner *Partner `json:"partner"`
	// Reasons 是命中的全部规则说明，按名称 → 邮箱 → 电话的固定顺序。
	Reasons []string `json:"reasons"`
}

// PartnerDuplicateGroup 是一组"可能是同一家"的已有主体。
type PartnerDuplicateGroup struct {
	// Rule 是这一组命中的规则，取值见 PartnerDup* 常量。
	Rule string `json:"rule"`
	// Reason 是给人看的命中说明，直接显示在列表里。
	Reason string `json:"reason"`
	// Partners 至少两条，按 id 升序（先建的在前，通常也是信息更全的那条）。
	Partners []Partner `json:"partners"`
}

// PartnerNameKey 把「名称 + 国家」归一成比较用的键，空串表示不参与名称查重。
//
// 归一化只做两件事：转小写、去掉字母数字以外的字符。这样 `Ferreteria El Sol`、
// `FERRETERIA  EL-SOL` 与 `FerreteriaElSol.` 是同一家；而刻意**不剥公司后缀**
// （Co., Ltd / 有限公司），因为 `XX 贸易` 与 `XX 有限公司` 完全可能是两个主体，
// 剥掉后缀会把它们并成一条，合并之后更难拆。
func PartnerNameKey(name, country string) string {
	n := normalizePartnerText(name)
	// 单字符名称（"A"、""）拿去分组只会把无关主体串在一起。
	if utf8.RuneCountInString(n) < 2 {
		return ""
	}
	return n + "|" + normalizePartnerText(country)
}

// PartnerPhoneKey 取电话数字部分的尾 9 位，空串表示不参与电话查重。
//
// 同一个号码在名片、邮件签名、报关资料上有四种写法：`+593 9 0000 0001`、
// `593900000001`、`0900000001`、`900000001`。尾 9 位能吃掉 1～4 位国家码，
// 又比"尾 4 位"有区分度——后者会把一个城市的座机并成同一家。
func PartnerPhoneKey(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	// 少于 7 位不参与：分机号、区号片段这类残号比出来的"重复"没有意义。
	if len(digits) < 7 {
		return ""
	}
	if len(digits) > 9 {
		digits = digits[len(digits)-9:]
	}
	return digits
}

// PartnerEmailDomainKey 取邮箱域名，免费邮箱与不成形的邮箱返回空串。
//
// 企业域名相同是很强的信号；而 gmail / qq / 163 这类免费邮箱人人可用，
// 拿它分组会把半个客户表并成一组，所以整张名单排除掉。
func PartnerEmailDomainKey(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ""
	}
	domain := strings.ToLower(strings.TrimSpace(email[at+1:]))
	domain = strings.TrimPrefix(domain, "www.")
	if !strings.Contains(domain, ".") || strings.ContainsAny(domain, " \t") {
		return ""
	}
	if freeMailDomains[domain] {
		return ""
	}
	return domain
}

// freeMailDomains 是"谁都能注册"的邮箱域，不参与查重。
//
// 名单只收常见服务商，宁可漏（少提示一次）也不要把 gmail 上的两个客户
// 判成同一家——误合并会把两家的跟进、订单、应收搅在一起，比漏提示难收拾得多。
var freeMailDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true,
	"qq.com": true, "foxmail.com": true, "vip.qq.com": true,
	"163.com": true, "126.com": true, "yeah.net": true, "188.com": true,
	"sina.com": true, "sohu.com": true, "139.com": true, "189.cn": true,
	"outlook.com": true, "hotmail.com": true, "live.com": true, "msn.com": true,
	"yahoo.com": true, "yahoo.com.cn": true, "yahoo.co.jp": true,
	"icloud.com": true, "me.com": true, "aol.com": true,
	"mail.ru": true, "yandex.ru": true, "proton.me": true, "protonmail.com": true,
	"gmx.com": true, "zoho.com": true,
}

// GroupPartnerDuplicates 把全部主体按三条规则各分一遍组，只返回 ≥2 条的分组。
//
// 输出顺序固定：先按规则（名称 → 邮箱 → 电话），同一规则内按首次出现的顺序
// （调用方按 id 升序传入）。不排序的话同一份数据两次调用可能给出不同的排列，
// 界面上的分组会莫名其妙地跳。
func GroupPartnerDuplicates(all []Partner) []PartnerDuplicateGroup {
	groups := make([]PartnerDuplicateGroup, 0)
	for _, rule := range partnerDupRules {
		order := make([]string, 0, len(all))
		buckets := make(map[string][]Partner, len(all))
		for i := range all {
			key := rule.key(&all[i])
			if key == "" {
				continue
			}
			if _, seen := buckets[key]; !seen {
				order = append(order, key)
			}
			buckets[key] = append(buckets[key], all[i])
		}
		for _, key := range order {
			bucket := buckets[key]
			if len(bucket) < 2 {
				continue
			}
			groups = append(groups, PartnerDuplicateGroup{
				Rule:     rule.name,
				Reason:   rule.reason(key, &bucket[0]),
				Partners: bucket,
			})
		}
	}
	return groups
}

// FindPartnerDuplicates 用 probe 的四要素去比对已有主体，一条主体只出现一次，
// 命中多条规则时把原因并进 Reasons。
//
// 只提示不阻断：外贸同名公司确实存在（不同国家的两家 "Sunrise Trading"），
// 硬拦会让人无法把一笔真实询盘落库。
func FindPartnerDuplicates(all []Partner, probe PartnerProbe) []PartnerDuplicateCandidate {
	nameKey := PartnerNameKey(probe.Name, probe.Country)
	phoneKey := PartnerPhoneKey(probe.Phone)
	domainKey := PartnerEmailDomainKey(probe.Email)
	if nameKey == "" && phoneKey == "" && domainKey == "" {
		return []PartnerDuplicateCandidate{}
	}

	out := make([]PartnerDuplicateCandidate, 0, 4)
	index := make(map[uint]int, 4)
	hit := func(p *Partner, reason string) {
		if i, ok := index[p.ID]; ok {
			out[i].Reasons = append(out[i].Reasons, reason)
			return
		}
		index[p.ID] = len(out)
		out = append(out, PartnerDuplicateCandidate{Partner: p, Reasons: []string{reason}})
	}

	for i := range all {
		p := &all[i]
		if probe.ExcludeID > 0 && p.ID == probe.ExcludeID {
			continue
		}
		if nameKey != "" && PartnerNameKey(p.Name, p.Country) == nameKey {
			hit(p, nameCountryReason("", p))
		}
		if domainKey != "" && PartnerEmailDomainKey(p.Email) == domainKey {
			hit(p, emailDomainReason(domainKey, p))
		}
		if phoneKey != "" && PartnerPhoneKey(p.Phone) == phoneKey {
			hit(p, phoneReason("", p))
		}
	}
	return out
}

// partnerDupRule 是一条规则的三个零件：名字、取键、把键翻成一句人话。
type partnerDupRule struct {
	name   string
	key    func(p *Partner) string
	reason func(key string, first *Partner) string
}

// partnerDupRules 的顺序就是输出顺序，也是理由的拼接顺序。
var partnerDupRules = []partnerDupRule{
	{
		name: PartnerDupNameCountry,
		key: func(p *Partner) string {
			return PartnerNameKey(p.Name, p.Country)
		},
		reason: nameCountryReason,
	},
	{
		name: PartnerDupEmailDomain,
		key: func(p *Partner) string {
			return PartnerEmailDomainKey(p.Email)
		},
		reason: emailDomainReason,
	},
	{
		name: PartnerDupPhone,
		key: func(p *Partner) string {
			return PartnerPhoneKey(p.Phone)
		},
		reason: phoneReason,
	},
}

func nameCountryReason(_ string, first *Partner) string {
	if strings.TrimSpace(first.Country) == "" {
		return "名称相同，且双方都没填国家：" + first.Name
	}
	return fmt.Sprintf("名称与国家相同：%s / %s", first.Name, first.Country)
}

func emailDomainReason(domain string, _ *Partner) string {
	return "邮箱同域：@" + domain
}

func phoneReason(_ string, first *Partner) string {
	return "电话尾号相同：" + first.Phone
}

// normalizePartnerText 转小写并去掉字母数字以外的字符（空格、连字符、点、括号等）。
//
// 用 unicode 的 IsLetter/IsDigit 而不是只保留 ASCII：中文名、越南文、
// 带重音的西班牙文客户名都要能归一，否则它们永远比不上。
func normalizePartnerText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
