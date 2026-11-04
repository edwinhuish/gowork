import { Navigate } from 'react-router'
import type { RouteObject } from 'react-router'

import { ActivityCreate, ActivityEdit, ActivityList } from '@/pages/activities'
import { AgingPage } from '@/pages/aging'
import { AuditLogList } from '@/pages/audit-logs'
import { ClauseList } from '@/pages/clauses'
import { CollectionPage } from '@/pages/collection'
import { ContactCreate, ContactEdit, ContactList } from '@/pages/contacts'
import { ContractCreate, ContractEdit, ContractList, ContractShow } from '@/pages/contracts'
import { CurrencyCreate, CurrencyEdit, CurrencyList } from '@/pages/currencies'
import { CustomerCreate, CustomerEdit, CustomerList, CustomerShow } from '@/pages/customers'
import { DashboardPage } from '@/pages/dashboard'
import { ExchangeGainsPage } from '@/pages/exchange-gains'
import { ExchangeRateCreate, ExchangeRateList } from '@/pages/exchange-rates'
import { ForwarderCreate, ForwarderEdit, ForwarderList, ForwarderShow } from '@/pages/forwarders'
import { LeadCreate, LeadEdit, LeadList } from '@/pages/leads'
import { OpportunityCreate, OpportunityEdit, OpportunityList, OpportunityShow } from '@/pages/opportunities'
import { OrderCreate, OrderEdit, OrderList, OrderShow } from '@/pages/orders'
import { PayableCreate, PayableEdit, PayableList, PayableShow } from '@/pages/payables'
import { PaymentCreate, PaymentEdit, PaymentList, PaymentShow } from '@/pages/payments'
import { ProductCreate, ProductEdit, ProductList, ProductShow } from '@/pages/products'
import { PurchaseCreate, PurchaseEdit, PurchaseList, PurchaseShow } from '@/pages/purchases'
import {
  QuotationCreate,
  QuotationEdit,
  QuotationList,
  QuotationShow,
} from '@/pages/quotations'
import { ReceivableCreate, ReceivableEdit, ReceivableList, ReceivableShow } from '@/pages/receivables'
import { SalesDashboardPage } from '@/pages/sales-dashboard'
import { ShipmentCreate, ShipmentEdit, ShipmentList, ShipmentShow } from '@/pages/shipments'
import { StockList } from '@/pages/stock'
import {
  StockDocumentCreate,
  StockDocumentEdit,
  StockDocumentList,
  StockDocumentShow,
} from '@/pages/stock-documents'
import { StockMovementCreate, StockMovementList } from '@/pages/stock-movements'
import { SupplierCreate, SupplierEdit, SupplierList, SupplierShow } from '@/pages/suppliers'
import { TodoPage } from '@/pages/todos'
import { UserAccountList } from '@/pages/user-accounts/list'
import { WarehouseList } from '@/pages/warehouses/list'

/**
 * 受保护区的页面路由表。
 *
 * 这里是**数据**而不是 JSX 树，因为每个标签页都要各自渲染一次：
 * 外壳（AppLayout）不再用 `<Outlet />`，而是给每个已打开的标签页
 * 用 `useRoutes(appRoutes, tab.location)` 单独匹配一遍，这样
 * `useParams` 拿到的是那个标签页自己的 :id，而不是当前地址栏的。
 *
 * 未匹配路径回首页：外壳在 `<Route path="*">` 上，任何地址都会进来，
 * 没有这一条兜底就会渲染出一片空白。
 */
export const appRoutes: RouteObject[] = [
  { index: true, element: <DashboardPage /> },

  { path: 'todos', element: <TodoPage /> },

  {
    path: 'products',
    children: [
      { index: true, element: <ProductList /> },
      { path: 'create', element: <ProductCreate /> },
      { path: 'edit/:id', element: <ProductEdit /> },
      { path: 'show/:id', element: <ProductShow /> },
    ],
  },

  {
    path: 'suppliers',
    children: [
      { index: true, element: <SupplierList /> },
      { path: 'create', element: <SupplierCreate /> },
      { path: 'edit/:id', element: <SupplierEdit /> },
      { path: 'show/:id', element: <SupplierShow /> },
    ],
  },

  {
    path: 'customers',
    children: [
      { index: true, element: <CustomerList /> },
      { path: 'create', element: <CustomerCreate /> },
      { path: 'edit/:id', element: <CustomerEdit /> },
      { path: 'show/:id', element: <CustomerShow /> },
    ],
  },

  {
    path: 'forwarders',
    children: [
      { index: true, element: <ForwarderList /> },
      { path: 'create', element: <ForwarderCreate /> },
      { path: 'edit/:id', element: <ForwarderEdit /> },
      { path: 'show/:id', element: <ForwarderShow /> },
    ],
  },

  {
    path: 'contacts',
    children: [
      { index: true, element: <ContactList /> },
      { path: 'create', element: <ContactCreate /> },
      { path: 'edit/:id', element: <ContactEdit /> },
    ],
  },

  {
    path: 'activities',
    children: [
      { index: true, element: <ActivityList /> },
      { path: 'create', element: <ActivityCreate /> },
      { path: 'edit/:id', element: <ActivityEdit /> },
    ],
  },

  {
    path: 'leads',
    children: [
      { index: true, element: <LeadList /> },
      { path: 'create', element: <LeadCreate /> },
      { path: 'edit/:id', element: <LeadEdit /> },
    ],
  },

  {
    path: 'opportunities',
    children: [
      { index: true, element: <OpportunityList /> },
      { path: 'create', element: <OpportunityCreate /> },
      { path: 'edit/:id', element: <OpportunityEdit /> },
      { path: 'show/:id', element: <OpportunityShow /> },
    ],
  },

  {
    path: 'quotations',
    children: [
      { index: true, element: <QuotationList /> },
      { path: 'create', element: <QuotationCreate /> },
      { path: 'edit/:id', element: <QuotationEdit /> },
      { path: 'show/:id', element: <QuotationShow /> },
    ],
  },

  {
    path: 'contracts',
    children: [
      { index: true, element: <ContractList /> },
      { path: 'create', element: <ContractCreate /> },
      { path: 'edit/:id', element: <ContractEdit /> },
      { path: 'show/:id', element: <ContractShow /> },
    ],
  },

  { path: 'clauses', element: <ClauseList /> },

  {
    path: 'sales-orders',
    children: [
      { index: true, element: <OrderList /> },
      { path: 'create', element: <OrderCreate /> },
      { path: 'edit/:id', element: <OrderEdit /> },
      { path: 'show/:id', element: <OrderShow /> },
    ],
  },

  {
    path: 'purchase-orders',
    children: [
      { index: true, element: <PurchaseList /> },
      { path: 'create', element: <PurchaseCreate /> },
      { path: 'edit/:id', element: <PurchaseEdit /> },
      { path: 'show/:id', element: <PurchaseShow /> },
    ],
  },

  {
    path: 'shipments',
    children: [
      { index: true, element: <ShipmentList /> },
      { path: 'create', element: <ShipmentCreate /> },
      { path: 'edit/:id', element: <ShipmentEdit /> },
      { path: 'show/:id', element: <ShipmentShow /> },
    ],
  },

  { path: 'stock', element: <StockList /> },

  {
    path: 'stock-documents',
    children: [
      { index: true, element: <StockDocumentList /> },
      { path: 'create', element: <StockDocumentCreate /> },
      { path: 'edit/:id', element: <StockDocumentEdit /> },
      { path: 'show/:id', element: <StockDocumentShow /> },
    ],
  },

  {
    path: 'stock-movements',
    children: [
      { index: true, element: <StockMovementList /> },
      { path: 'create', element: <StockMovementCreate /> },
    ],
  },

  {
    path: 'receivables',
    children: [
      { index: true, element: <ReceivableList /> },
      { path: 'create', element: <ReceivableCreate /> },
      { path: 'edit/:id', element: <ReceivableEdit /> },
      { path: 'show/:id', element: <ReceivableShow /> },
    ],
  },

  {
    path: 'payables',
    children: [
      { index: true, element: <PayableList /> },
      { path: 'create', element: <PayableCreate /> },
      { path: 'edit/:id', element: <PayableEdit /> },
      { path: 'show/:id', element: <PayableShow /> },
    ],
  },

  {
    path: 'payments',
    children: [
      { index: true, element: <PaymentList /> },
      { path: 'create', element: <PaymentCreate /> },
      { path: 'edit/:id', element: <PaymentEdit /> },
      { path: 'show/:id', element: <PaymentShow /> },
    ],
  },

  { path: 'aging', element: <AgingPage /> },
  { path: 'exchange-gains', element: <ExchangeGainsPage /> },
  { path: 'collection', element: <CollectionPage /> },
  { path: 'sales-dashboard', element: <SalesDashboardPage /> },

  {
    path: 'currencies',
    children: [
      { index: true, element: <CurrencyList /> },
      { path: 'create', element: <CurrencyCreate /> },
      { path: 'edit/:id', element: <CurrencyEdit /> },
    ],
  },

  {
    path: 'exchange-rates',
    children: [
      { index: true, element: <ExchangeRateList /> },
      { path: 'create', element: <ExchangeRateCreate /> },
    ],
  },

  { path: 'warehouses', element: <WarehouseList /> },
  { path: 'user-accounts', element: <UserAccountList /> },
  { path: 'audit-logs', element: <AuditLogList /> },

  { path: '*', element: <Navigate to="/" replace /> },
]
