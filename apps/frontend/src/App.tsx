import {
  AccountBookOutlined,
  BookOutlined,
  AlertOutlined,
  AppstoreOutlined,
  BellOutlined,
  BarChartOutlined,
  LineChartOutlined,
  CommentOutlined,
  ContactsOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DollarOutlined,
  FileProtectOutlined,
  FileSearchOutlined,
  FileTextOutlined,
  FundOutlined,
  GlobalOutlined,
  HistoryOutlined,
  ImportOutlined,
  PieChartOutlined,
  ProfileOutlined,
  SendOutlined,
  ShoppingCartOutlined,
  ShopOutlined,
  UserAddOutlined,
  UserOutlined,
  SwapOutlined,
  TeamOutlined,
  TransactionOutlined,
  WalletOutlined,
} from '@ant-design/icons'
import { Authenticated, Refine } from '@refinedev/core'
import routerProvider, {
  CatchAllNavigate,
  DocumentTitleHandler,
  NavigateToResource,
  UnsavedChangesNotifier,
} from '@refinedev/react-router'
import { App as AntApp, ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { BrowserRouter, Navigate, Outlet, Route, Routes } from 'react-router'

import { AppLayout } from '@/components/layout/AppLayout'
import { ActivityCreate, ActivityEdit, ActivityList } from '@/pages/activities'
import { AgingPage } from '@/pages/aging'
import { ExchangeGainsPage } from '@/pages/exchange-gains'
import { ContactCreate, ContactEdit, ContactList } from '@/pages/contacts'
import { AuditLogList } from '@/pages/audit-logs'
import { UserAccountList } from '@/pages/user-accounts/list'
import { WarehouseList } from '@/pages/warehouses/list'
import { ClauseList } from '@/pages/clauses'
import { CollectionPage } from '@/pages/collection'
import { ContractCreate, ContractEdit, ContractList, ContractShow } from '@/pages/contracts'
import { CurrencyCreate, CurrencyEdit, CurrencyList } from '@/pages/currencies'
import { CustomerCreate, CustomerEdit, CustomerList, CustomerShow } from '@/pages/customers'
import { DashboardPage } from '@/pages/dashboard'
import { ExchangeRateCreate, ExchangeRateList } from '@/pages/exchange-rates'
import { ForwarderCreate, ForwarderEdit, ForwarderList, ForwarderShow } from '@/pages/forwarders'
import { LeadCreate, LeadEdit, LeadList } from '@/pages/leads'
import { LoginPage } from '@/pages/login'
import { OpportunityCreate, OpportunityEdit, OpportunityList, OpportunityShow } from '@/pages/opportunities'
import { PayableCreate, PayableEdit, PayableList, PayableShow } from '@/pages/payables'
import { PaymentCreate, PaymentEdit, PaymentList, PaymentShow } from '@/pages/payments'
import { ProductCreate, ProductEdit, ProductList, ProductShow } from '@/pages/products'
import {
  QuotationCreate,
  QuotationEdit,
  QuotationList,
  QuotationShow,
} from '@/pages/quotations'
import { OrderCreate, OrderEdit, OrderList, OrderShow } from '@/pages/orders'
import {
  ReceivableCreate,
  ReceivableEdit,
  ReceivableList,
  ReceivableShow,
} from '@/pages/receivables'
import {
  PurchaseCreate,
  PurchaseEdit,
  PurchaseList,
  PurchaseShow,
} from '@/pages/purchases'
import {
  ShipmentCreate,
  ShipmentEdit,
  ShipmentList,
  ShipmentShow,
} from '@/pages/shipments'
import { StockList } from '@/pages/stock'
import {
  StockDocumentCreate,
  StockDocumentEdit,
  StockDocumentList,
  StockDocumentShow,
} from '@/pages/stock-documents'
import { StockMovementCreate, StockMovementList } from '@/pages/stock-movements'
import { TodoPage } from '@/pages/todos'
import { SalesDashboardPage } from '@/pages/sales-dashboard'
import { SupplierCreate, SupplierEdit, SupplierList, SupplierShow } from '@/pages/suppliers'
import { authProvider } from '@/providers/authProvider'
import { dataProvider } from '@/providers/dataProvider'
import { themeConfig } from '@/theme'

/**
 * 应用根。
 *
 * 三段式路由，是 refine 的标准鉴权布局：
 *   1 受保护段：Authenticated 未登录时 fallback 到 /login，登录后渲染 AppLayout
 *   2 登录段：已登录访问 /login 时 NavigateToResource 把人送回首页
 *   3 兜底：未知路径回首页
 *
 * resources 是导航与面包屑的唯一来源 —— 加一个模块只需要在这里加一条，
 * 侧边栏菜单、页面标题、返回链接都会跟着变。
 */
export default function App() {
  return (
    <BrowserRouter>
      <ConfigProvider locale={zhCN} theme={themeConfig}>
        <AntApp>
          <Refine
            dataProvider={dataProvider}
            authProvider={authProvider}
            routerProvider={routerProvider}
            resources={[
              {
                name: 'dashboard',
                list: '/',
                meta: { label: '概览', icon: <DashboardOutlined /> },
              },
              {
                // 待办：待跟进、应收待催、缺当日汇率。三类各有明确口径，页面上写清。
                name: 'todos',
                list: '/todos',
                meta: { label: '待办', icon: <BellOutlined /> },
              },
              {
                name: 'products',
                list: '/products',
                create: '/products/create',
                edit: '/products/edit/:id',
                show: '/products/show/:id',
                meta: { label: '产品', icon: <AppstoreOutlined /> },
              },
              {
                name: 'suppliers',
                list: '/suppliers',
                create: '/suppliers/create',
                edit: '/suppliers/edit/:id',
                show: '/suppliers/show/:id',
                meta: { label: '供应商', icon: <ShopOutlined /> },
              },
              {
                name: 'customers',
                list: '/customers',
                create: '/customers/create',
                edit: '/customers/edit/:id',
                show: '/customers/show/:id',
                meta: { label: '客户', icon: <TeamOutlined /> },
              },
              {
                name: 'forwarders',
                list: '/forwarders',
                create: '/forwarders/create',
                edit: '/forwarders/edit/:id',
                show: '/forwarders/show/:id',
                meta: { label: '货代', icon: <GlobalOutlined /> },
              },
              {
                name: 'contacts',
                list: '/contacts',
                create: '/contacts/create',
                edit: '/contacts/edit/:id',
                meta: { label: '联系人', icon: <ContactsOutlined /> },
              },
              {
                name: 'activities',
                list: '/activities',
                create: '/activities/create',
                edit: '/activities/edit/:id',
                meta: { label: '跟进记录', icon: <CommentOutlined /> },
              },
              {
                name: 'leads',
                list: '/leads',
                create: '/leads/create',
                edit: '/leads/edit/:id',
                meta: { label: '线索', icon: <UserAddOutlined /> },
              },
              {
                name: 'opportunities',
                list: '/opportunities',
                create: '/opportunities/create',
                edit: '/opportunities/edit/:id',
                show: '/opportunities/show/:id',
                meta: { label: '商机', icon: <FundOutlined /> },
              },
              {
                // 报价单是 ERP 侧第一张单据：单号、状态机、成本快照都从它开始。
                name: 'quotations',
                list: '/quotations',
                create: '/quotations/create',
                edit: '/quotations/edit/:id',
                show: '/quotations/show/:id',
                meta: { label: '报价单', icon: <FileTextOutlined /> },
              },
              {
                // 合同：PI 与销售合同共用一张表。没有空白新建，
                // 只能从已确认的报价单或订单生成，金额与汇率照抄来源。
                name: 'contracts',
                list: '/contracts',
                create: '/contracts/create',
                edit: '/contracts/edit/:id',
                show: '/contracts/show/:id',
                meta: { label: '合同', icon: <FileProtectOutlined /> },
              },
              {
                // 条款模板库（T-27）：主数据，合同生成时抄成快照。
                name: 'clauses',
                list: '/clauses',
                meta: { label: '条款库', icon: <BookOutlined /> },
              },
              {
                // 订单：报价谈成之后要出货的那张凭证，状态走完整状态机。
                name: 'sales-orders',
                list: '/sales-orders',
                create: '/sales-orders/create',
                edit: '/sales-orders/edit/:id',
                show: '/sales-orders/show/:id',
                meta: { label: '销售订单', icon: <ProfileOutlined /> },
              },
              {
                // 采购订单：销售订单的镜像，明细可挂销售单行做「以销定采」。
                name: 'purchase-orders',
                list: '/purchase-orders',
                create: '/purchase-orders/create',
                edit: '/purchase-orders/edit/:id',
                show: '/purchase-orders/show/:id',
                meta: { label: '采购订单', icon: <ShoppingCartOutlined /> },
              },
              {
                // 出货批次：一票装哪些销售订单行。与订单是多对多（分批出、拼柜出）。
                name: 'shipments',
                list: '/shipments',
                create: '/shipments/create',
                edit: '/shipments/edit/:id',
                show: '/shipments/show/:id',
                meta: { label: '出货单', icon: <SendOutlined /> },
              },
              {
                // 库存只有读与重算：能改库存的只有流水，所以这里没有 create / edit。
                name: 'stock-snapshots',
                list: '/stock',
                meta: { label: '库存', icon: <DatabaseOutlined /> },
              },
              {
                // 出入库单：草稿不动库存，确认那一刻才生成流水。
                name: 'stock-documents',
                list: '/stock-documents',
                create: '/stock-documents/create',
                edit: '/stock-documents/edit/:id',
                show: '/stock-documents/show/:id',
                meta: { label: '出入库单', icon: <ImportOutlined /> },
              },
              {
                name: 'stock-movements',
                list: '/stock-movements',
                create: '/stock-movements/create',
                meta: { label: '库存流水', icon: <HistoryOutlined /> },
              },
              {
                name: 'receivables',
                list: '/receivables',
                create: '/receivables/create',
                edit: '/receivables/edit/:id',
                show: '/receivables/show/:id',
                meta: { label: '应收', icon: <AccountBookOutlined /> },
              },
              {
                name: 'payables',
                list: '/payables',
                create: '/payables/create',
                edit: '/payables/edit/:id',
                show: '/payables/show/:id',
                meta: { label: '应付', icon: <WalletOutlined /> },
              },
              {
                name: 'payments',
                list: '/payments',
                create: '/payments/create',
                edit: '/payments/edit/:id',
                show: '/payments/show/:id',
                meta: { label: '收付款', icon: <TransactionOutlined /> },
              },
              {
                // 账龄是只读聚合：没有 create / edit，只有一页报表。
                name: 'aging',
                list: '/aging',
                meta: { label: '账龄', icon: <BarChartOutlined /> },
              },
              {
                name: 'exchange-gains',
                list: '/exchange-gains',
                meta: { label: '汇兑损益', icon: <LineChartOutlined /> },
              },
              {
                // 回款与超期：期间回款率是流量，超期明细是存量，两者不互相校验。
                name: 'collection',
                list: '/collection',
                meta: { label: '回款与超期', icon: <AlertOutlined /> },
              },
              {
                // 销售看板：按客户 / 产品 / 国家 / 月汇总，口径与单票毛利同源。
                name: 'sales-dashboard',
                list: '/sales-dashboard',
                meta: { label: '销售看板', icon: <PieChartOutlined /> },
              },
              {
                name: 'currencies',
                list: '/currencies',
                create: '/currencies/create',
                edit: '/currencies/edit/:id',
                meta: { label: '币种', icon: <DollarOutlined /> },
              },
              {
                // 汇率没有 edit：历史汇率不可改，所以 resources 里也不声明编辑路径。
                name: 'exchange-rates',
                list: '/exchange-rates',
                create: '/exchange-rates/create',
                meta: { label: '汇率', icon: <SwapOutlined /> },
              },
              {
                // 仓库（T-56）：主数据维护，建仓与设默认。
                name: 'warehouses',
                list: '/warehouses',
                meta: { label: '仓库', icon: <ShopOutlined /> },
              },
              {
                // 账号维护同样只有管理员能用；菜单对所有人可见，权限由接口兜底。
                name: 'user-accounts',
                list: '/user-accounts',
                meta: { label: '账号', icon: <UserOutlined /> },
              },
              {
                // 操作日志只读、且只有管理员能查：里面装着全公司单据的提交内容。
                name: 'audit-logs',
                list: '/audit-logs',
                meta: { label: '操作日志', icon: <FileSearchOutlined /> },
              },
            ]}
            options={{
              syncWithLocation: true,
              warnWhenUnsavedChanges: true,
              title: { text: 'GoWork ERP · CRM' },
            }}
          >
            <Routes>
              <Route
                element={
                  <Authenticated key="protected-routes" fallback={<CatchAllNavigate to="/login" />}>
                    <Outlet />
                  </Authenticated>
                }
              >
                <Route element={<AppLayout />}>
                  <Route index element={<DashboardPage />} />

                  <Route path="todos">
                    <Route index element={<TodoPage />} />
                  </Route>

                  <Route path="products">
                    <Route index element={<ProductList />} />
                    <Route path="create" element={<ProductCreate />} />
                    <Route path="edit/:id" element={<ProductEdit />} />
                    <Route path="show/:id" element={<ProductShow />} />
                  </Route>

                  <Route path="suppliers">
                    <Route index element={<SupplierList />} />
                    <Route path="create" element={<SupplierCreate />} />
                    <Route path="edit/:id" element={<SupplierEdit />} />
                    <Route path="show/:id" element={<SupplierShow />} />
                  </Route>

                  <Route path="customers">
                    <Route index element={<CustomerList />} />
                    <Route path="create" element={<CustomerCreate />} />
                    <Route path="edit/:id" element={<CustomerEdit />} />
                    <Route path="show/:id" element={<CustomerShow />} />
                  </Route>

                  <Route path="forwarders">
                    <Route index element={<ForwarderList />} />
                    <Route path="create" element={<ForwarderCreate />} />
                    <Route path="edit/:id" element={<ForwarderEdit />} />
                    <Route path="show/:id" element={<ForwarderShow />} />
                  </Route>

                  <Route path="contacts">
                    <Route index element={<ContactList />} />
                    <Route path="create" element={<ContactCreate />} />
                    <Route path="edit/:id" element={<ContactEdit />} />
                  </Route>

                  <Route path="activities">
                    <Route index element={<ActivityList />} />
                    <Route path="create" element={<ActivityCreate />} />
                    <Route path="edit/:id" element={<ActivityEdit />} />
                  </Route>

                  <Route path="leads">
                    <Route index element={<LeadList />} />
                    <Route path="create" element={<LeadCreate />} />
                    <Route path="edit/:id" element={<LeadEdit />} />
                  </Route>

                  <Route path="opportunities">
                    <Route path="show/:id" element={<OpportunityShow />} />
                    <Route index element={<OpportunityList />} />
                    <Route path="create" element={<OpportunityCreate />} />
                    <Route path="edit/:id" element={<OpportunityEdit />} />
                  </Route>

                  <Route path="quotations">
                    <Route index element={<QuotationList />} />
                    <Route path="create" element={<QuotationCreate />} />
                    <Route path="edit/:id" element={<QuotationEdit />} />
                    <Route path="show/:id" element={<QuotationShow />} />
                  </Route>

                  <Route path="contracts">
                    <Route index element={<ContractList />} />
                    <Route path="create" element={<ContractCreate />} />
                    <Route path="edit/:id" element={<ContractEdit />} />
                    <Route path="show/:id" element={<ContractShow />} />
                  </Route>

                  <Route path="clauses">
                    <Route index element={<ClauseList />} />
                  </Route>

                  <Route path="sales-orders">
                    <Route index element={<OrderList />} />
                    <Route path="create" element={<OrderCreate />} />
                    <Route path="edit/:id" element={<OrderEdit />} />
                    <Route path="show/:id" element={<OrderShow />} />
                  </Route>

                  <Route path="purchase-orders">
                    <Route index element={<PurchaseList />} />
                    <Route path="create" element={<PurchaseCreate />} />
                    <Route path="edit/:id" element={<PurchaseEdit />} />
                    <Route path="show/:id" element={<PurchaseShow />} />
                  </Route>

                  <Route path="shipments">
                    <Route index element={<ShipmentList />} />
                    <Route path="create" element={<ShipmentCreate />} />
                    <Route path="edit/:id" element={<ShipmentEdit />} />
                    <Route path="show/:id" element={<ShipmentShow />} />
                  </Route>

                  <Route path="stock">
                    <Route index element={<StockList />} />
                  </Route>

                  <Route path="stock-documents">
                    <Route index element={<StockDocumentList />} />
                    <Route path="create" element={<StockDocumentCreate />} />
                    <Route path="edit/:id" element={<StockDocumentEdit />} />
                    <Route path="show/:id" element={<StockDocumentShow />} />
                  </Route>

                  <Route path="stock-movements">
                    <Route index element={<StockMovementList />} />
                    <Route path="create" element={<StockMovementCreate />} />
                  </Route>

                  <Route path="receivables">
                    <Route index element={<ReceivableList />} />
                    <Route path="create" element={<ReceivableCreate />} />
                    <Route path="edit/:id" element={<ReceivableEdit />} />
                    <Route path="show/:id" element={<ReceivableShow />} />
                  </Route>

                  <Route path="payables">
                    <Route index element={<PayableList />} />
                    <Route path="create" element={<PayableCreate />} />
                    <Route path="edit/:id" element={<PayableEdit />} />
                    <Route path="show/:id" element={<PayableShow />} />
                  </Route>

                  <Route path="payments">
                    <Route index element={<PaymentList />} />
                    <Route path="create" element={<PaymentCreate />} />
                    <Route path="edit/:id" element={<PaymentEdit />} />
                    <Route path="show/:id" element={<PaymentShow />} />
                  </Route>

                  <Route path="aging">
                    <Route index element={<AgingPage />} />
                  </Route>

                  <Route path="exchange-gains">
                    <Route index element={<ExchangeGainsPage />} />
                  </Route>

                  <Route path="collection">
                    <Route index element={<CollectionPage />} />
                  </Route>

                  <Route path="sales-dashboard">
                    <Route index element={<SalesDashboardPage />} />
                  </Route>

                  <Route path="currencies">
                    <Route index element={<CurrencyList />} />
                    <Route path="create" element={<CurrencyCreate />} />
                    <Route path="edit/:id" element={<CurrencyEdit />} />
                  </Route>

                  <Route path="exchange-rates">
                    <Route index element={<ExchangeRateList />} />
                    <Route path="create" element={<ExchangeRateCreate />} />
                  </Route>

                  <Route path="warehouses">
                    <Route index element={<WarehouseList />} />
                  </Route>

                  <Route path="user-accounts">
                    <Route index element={<UserAccountList />} />
                  </Route>

                  <Route path="audit-logs">
                    <Route index element={<AuditLogList />} />
                  </Route>
                </Route>
              </Route>

              <Route
                element={
                  <Authenticated key="auth-routes" fallback={<Outlet />}>
                    <NavigateToResource resource="dashboard" />
                  </Authenticated>
                }
              >
                <Route path="login" element={<LoginPage />} />
              </Route>

              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>

            <UnsavedChangesNotifier />
            <DocumentTitleHandler />
          </Refine>
        </AntApp>
      </ConfigProvider>
    </BrowserRouter>
  )
}
