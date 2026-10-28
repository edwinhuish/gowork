package bootstrap

import (
	"context"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
	"github.com/edwinhuish/gowork/internal/service"
)

const defaultAdminPassword = "admin123"

// seed 只在数据为空时写入，重复启动不会产生重复数据。
func (a *App) seed(
	ctx context.Context,
	auth service.AuthService,
	users repository.UserRepository,
	partners repository.PartnerRepository,
	products repository.ProductRepository,
) error {
	if err := a.seedAdmin(ctx, auth, users); err != nil {
		return err
	}
	return a.seedDemoData(ctx, partners, products)
}

func (a *App) seedAdmin(ctx context.Context, auth service.AuthService, users repository.UserRepository) error {
	n, err := users.Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	hash, err := auth.HashPassword(defaultAdminPassword)
	if err != nil {
		return err
	}
	if err := users.Create(ctx, &model.User{
		Username: "admin",
		Password: hash,
		Nickname: "管理员",
		Role:     model.RoleAdmin,
		Status:   model.EnableStatusOn,
	}); err != nil {
		return err
	}

	a.Log.Warn("已创建默认管理员账号，请尽快修改密码",
		"username", "admin", "password", defaultAdminPassword)
	return nil
}

// seedDemoData 写入演示用的往来单位与产品。
//
// 演示数据刻意包含两种"合并前无法表达"的记录：
// 客供两用（role=3）与个人主体（entity_type=1），
// 这样列表、筛选、概览计数一跑起来就能看出新模型是否真的生效。
func (a *App) seedDemoData(
	ctx context.Context,
	partners repository.PartnerRepository,
	products repository.ProductRepository,
) error {
	n, err := partners.Count(ctx)
	if err != nil || n > 0 {
		return err
	}

	demoPartners := []model.Partner{
		{
			Code: "SUP001", Name: "永康市华信五金厂",
			Role: model.PartnerRoleSupplier, EntityType: model.PartnerEntityCompany,
			ContactName: "陈先生", Phone: "13800000001",
			Address: "浙江省金华市永康市", Status: model.EnableStatusOn,
		},
		{
			Code: "SUP002", Name: "揭阳铭泰不锈钢制品",
			Role: model.PartnerRoleSupplier, EntityType: model.PartnerEntityCompany,
			ContactName: "林小姐", Phone: "13800000002",
			Address: "广东省揭阳市", Status: model.EnableStatusOn,
		},
		{
			Code: "CUS001", Name: "Ferreteria El Sol",
			Role: model.PartnerRoleCustomer, EntityType: model.PartnerEntityCompany,
			Country: "Ecuador", ContactName: "Carlos", Phone: "+593 9 0000 0001",
			Address: "Guayaquil", Status: model.EnableStatusOn,
		},
		{
			// 客供两用：既买成品，又把包装物料卖回给我们。
			// 合并前这种主体必须在两张表各建一条、维护两套联系方式。
			Code: "BOTH001", Name: "宁波华翔工贸",
			Role:       model.PartnerRoleCustomer | model.PartnerRoleSupplier,
			EntityType: model.PartnerEntityCompany,
			Country:    "China", ContactName: "周经理", Phone: "13800000003",
			Address: "浙江省宁波市", Status: model.EnableStatusOn,
		},
		{
			// 个人主体：货代买手，报关资料与公司不同，所以单独标记类型。
			Code: "PER001", Name: "王海（个人买手）",
			Role: model.PartnerRoleCustomer, EntityType: model.PartnerEntityPerson,
			Country: "Ecuador", ContactName: "王海", Phone: "+593 9 0000 0002",
			Remark: "个人收款的买手，报关资料用个人身份", Status: model.EnableStatusOn,
		},
	}
	for i := range demoPartners {
		if err := partners.Create(ctx, &demoPartners[i]); err != nil {
			return err
		}
	}

	supplierID := demoPartners[0].ID
	demoProducts := []model.Product{
		{SKUCode: "BH01-0001-SS", NameCN: "304 不锈钢合页 4 寸", NameEN: "SS Hinge 4\"", Category: "BH", Spec: "100×75×3 mm", Unit: "PCS", SupplierID: &supplierID, CostPrice: 6.8, SalePrice: 9.9, Status: model.EnableStatusOn},
		{SKUCode: "BH01-0002-CP", NameCN: "铁合页 4 寸 镀铬", NameEN: "Iron Hinge 4\" CP", Category: "BH", Spec: "100×75×2.5 mm", Unit: "PCS", SupplierID: &supplierID, CostPrice: 3.2, SalePrice: 5.5, Status: model.EnableStatusOn},
		{SKUCode: "LK01-0001-BK", NameCN: "球形门锁 黑色", NameEN: "Round Knob Lock BK", Category: "LK", Spec: "60 mm", Unit: "SET", SupplierID: &supplierID, CostPrice: 12.5, SalePrice: 18.0, Status: model.EnableStatusOn},
		{SKUCode: "FS01-0001", NameCN: "自攻螺丝 4×30 十字", NameEN: "Self Tapping Screw 4×30", Category: "FS", Spec: "4×30 mm / 1000pcs", Unit: "BOX", SupplierID: &supplierID, CostPrice: 8.0, SalePrice: 12.0, Status: model.EnableStatusOn},
	}
	for i := range demoProducts {
		if err := products.Create(ctx, &demoProducts[i]); err != nil {
			return err
		}
	}

	a.Log.Info("已写入演示数据", "partners", len(demoPartners), "products", len(demoProducts))
	return nil
}
