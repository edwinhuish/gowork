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
	suppliers repository.SupplierRepository,
	customers repository.CustomerRepository,
	products repository.ProductRepository,
) error {
	if err := a.seedAdmin(ctx, auth, users); err != nil {
		return err
	}
	return a.seedDemoData(ctx, suppliers, customers, products)
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

func (a *App) seedDemoData(
	ctx context.Context,
	suppliers repository.SupplierRepository,
	customers repository.CustomerRepository,
	products repository.ProductRepository,
) error {
	n, err := suppliers.Count(ctx)
	if err != nil || n > 0 {
		return err
	}

	demoSuppliers := []model.Supplier{
		{Code: "SUP001", Name: "永康市华信五金厂", ContactName: "陈先生", Phone: "13800000001", Address: "浙江省金华市永康市", Status: model.EnableStatusOn},
		{Code: "SUP002", Name: "揭阳铭泰不锈钢制品", ContactName: "林小姐", Phone: "13800000002", Address: "广东省揭阳市", Status: model.EnableStatusOn},
	}
	for i := range demoSuppliers {
		if err := suppliers.Create(ctx, &demoSuppliers[i]); err != nil {
			return err
		}
	}

	demoCustomers := []model.Customer{
		{Code: "CUS001", Name: "Ferreteria El Sol", Country: "Ecuador", ContactName: "Carlos", Phone: "+593 9 0000 0001", Address: "Guayaquil", Status: model.EnableStatusOn},
	}
	for i := range demoCustomers {
		if err := customers.Create(ctx, &demoCustomers[i]); err != nil {
			return err
		}
	}

	superID := demoSuppliers[0].ID
	demoProducts := []model.Product{
		{SKUCode: "BH01-0001-SS", NameCN: "304 不锈钢合页 4 寸", NameEN: "SS Hinge 4\"", Category: "BH", Spec: "100×75×3 mm", Unit: "PCS", SupplierID: &superID, CostPrice: 6.8, SalePrice: 9.9, Status: model.EnableStatusOn},
		{SKUCode: "BH01-0002-CP", NameCN: "铁合页 4 寸 镀铬", NameEN: "Iron Hinge 4\" CP", Category: "BH", Spec: "100×75×2.5 mm", Unit: "PCS", SupplierID: &superID, CostPrice: 3.2, SalePrice: 5.5, Status: model.EnableStatusOn},
		{SKUCode: "LK01-0001-BK", NameCN: "球形门锁 黑色", NameEN: "Round Knob Lock BK", Category: "LK", Spec: "60 mm", Unit: "SET", SupplierID: &superID, CostPrice: 12.5, SalePrice: 18.0, Status: model.EnableStatusOn},
		{SKUCode: "FS01-0001", NameCN: "自攻螺丝 4×30 十字", NameEN: "Self Tapping Screw 4×30", Category: "FS", Spec: "4×30 mm / 1000pcs", Unit: "BOX", SupplierID: &superID, CostPrice: 8.0, SalePrice: 12.0, Status: model.EnableStatusOn},
	}
	for i := range demoProducts {
		if err := products.Create(ctx, &demoProducts[i]); err != nil {
			return err
		}
	}

	a.Log.Info("已写入演示数据", "suppliers", len(demoSuppliers), "customers", len(demoCustomers), "products", len(demoProducts))
	return nil
}
