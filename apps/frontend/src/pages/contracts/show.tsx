import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useInvalidate, useShow } from '@refinedev/core'
import { EditButton, Show } from '@refinedev/antd'
import {
  Button,
  Descriptions,
  Divider,
  message,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd'

import { download } from '@/api/client'
import { contractApi } from '@/api/contracts'
import { AttachmentPanel } from '@/components/attachment-panel'
import { DocChain } from '@/components/doc-chain'
import { Empty, Mono, Money, OptionTag } from '@/components/fields'
import { ResponsiveTable } from '@/components/page'
import {
  CONTRACT_STATUS_OPTIONS,
  CONTRACT_TYPE_OPTIONS,
  contractSourceTypeLabel,
  contractTypeLabel,
} from '@/constants/catalog'
import type { Contract, ContractClause, ContractTerm } from '@/types'
import { formatDate } from '@/utils/format'

/**
 * 合同详情。
 *
 * 只读：金额、汇率与分期都是生成时锁定的快照。这里能做的只有两件事——
 * 把草稿确认为承诺，或取消它；要改条款请回编辑页（仅草稿）。
 */
export function ContractShow() {
  const { result, query } = useShow<Contract>({ resource: 'contracts' })
  const invalidate = useInvalidate()
  const navigate = useNavigate()
  const [acting, setActing] = useState(false)
  const [printing, setPrinting] = useState(false)

  const c = result
  const terms = c?.terms ?? []
  // 条款快照：只有详情带（列表不带）。
  const clauses = c?.clauses ?? []
  // 金额是定点字符串，这里的合计只用于「分期是否与总额相符」的显示判断；
  // 服务端保存时已经按末位判过一次，前端不再要求逐位相等。
  const termsTotal = terms.reduce((acc, t) => acc + Number(t.amount), 0)
  const termsBaseTotal = terms.reduce((acc, t) => acc + Number(t.amount_base), 0)
  const matched =
    c !== undefined && Math.abs(termsTotal - Number(c.amount_total)) < 0.005

  const refresh = () => invalidate({ resource: 'contracts', invalidates: ['detail', 'list'] })

  const changeStatus = async (status: number, okText: string) => {
    if (!c) return
    setActing(true)
    try {
      await contractApi.updateStatus(c.id, status)
      message.success(okText)
      await refresh()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setActing(false)
    }
  }

  const openSource = () => {
    if (!c) return
    const path =
      c.source_type === 20
        ? `/sales-orders/show/${c.source_id}`
        : `/quotations/show/${c.source_id}`
    navigate(path)
  }

  // 草稿也能打印：PI 常常是先发出去给对方确认，再回来点确认。
  const printPDF = async () => {
    if (!c) return
    setPrinting(true)
    try {
      await download(`/contracts/${c.id}/pdf`, `${c.doc_no}.pdf`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '下载失败')
    } finally {
      setPrinting(false)
    }
  }

  // 给某一期开票（T-61）：金额、币种、汇率都取合同上的值，不重算。
  const invoice = async (term: ContractTerm) => {
    if (!c) return
    try {
      const r = await contractApi.invoiceTerm(c.id, term.id)
      message.success(`已生成应收 ${r.doc_no}`)
      await query.refetch()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '开票失败')
    }
  }

  return (
    <Show
      isLoading={query.isLoading}
      title={c ? `${c.doc_no} · ${contractTypeLabel(c.type)}` : '合同详情'}
      headerButtons={
        <Space wrap>
          <Button onClick={printPDF} loading={printing}>
            打印 PDF
          </Button>
          {c?.status === 10 && <EditButton resource="contracts" recordItemId={c.id} />}
          {c?.status === 10 && (
            <Popconfirm
              title="确认这份合同？"
              description="确认后金额与分期锁定、不可修改，只能取消"
              okText="确认"
              cancelText="再想想"
              onConfirm={() => changeStatus(20, '合同已确认')}
            >
              <Button type="primary" loading={acting}>
                确认合同
              </Button>
            </Popconfirm>
          )}
          {(c?.status === 10 || c?.status === 20) && (
            <Popconfirm
              title="取消这份合同？"
              description="取消是终态，记录保留供追溯，但不再参与统计"
              okText="取消合同"
              cancelText="返回"
              onConfirm={() => changeStatus(90, '合同已取消')}
            >
              <Button danger loading={acting}>
                取消合同
              </Button>
            </Popconfirm>
          )}
        </Space>
      }
    >
      <DocChain
        current="contract"
        docs={{
          contract: { docNo: c?.doc_no },
          ...(c?.source_type === 10
            ? { quotation: { docNo: c.source_doc_no, href: `/quotations/show/${c.source_id}` } }
            : {}),
          ...(c?.source_type === 20
            ? { order: { docNo: c.source_doc_no, href: `/sales-orders/show/${c.source_id}` } }
            : {}),
        }}
      />

      <Descriptions column={{ xs: 1, sm: 2, lg: 3 }} bordered size="small">
        <Descriptions.Item label="合同类型">
          {/* 类型没有"好 / 坏"，是身份，所以中性；状态走字典自带的 tone。 */}
          <OptionTag value={c?.type} options={CONTRACT_TYPE_OPTIONS} />
        </Descriptions.Item>
        <Descriptions.Item label="状态">
          <OptionTag value={c?.status} options={CONTRACT_STATUS_OPTIONS} />
        </Descriptions.Item>
        <Descriptions.Item label="客户">{c?.partner?.name || <Empty />}</Descriptions.Item>
        <Descriptions.Item label="来源单据">
          {c ? (
            <Space size={6}>
              <Tag>{contractSourceTypeLabel(c.source_type)}</Tag>
              <Button type="link" size="small" style={{ padding: 0 }} onClick={openSource}>
                {c.source_doc_no}
              </Button>
            </Space>
          ) : (
            <Empty />
          )}
        </Descriptions.Item>
        <Descriptions.Item label="签订日">{formatDate(c?.sign_date)}</Descriptions.Item>
        <Descriptions.Item label="约定交期">
          {formatDate(c?.delivery_date ?? undefined)}
        </Descriptions.Item>
        <Descriptions.Item label="币种 / 汇率">
          {c ? (
            <Space size={6}>
              <Tag>{c.currency_code}</Tag>
              <span>{c.exchange_rate}</span>
            </Space>
          ) : (
            <Empty />
          )}
        </Descriptions.Item>
        <Descriptions.Item label="原币总额">
          {c ? (
            <Space size={4}>
              <Money value={c.amount_total} />
              <Tag>{c.currency_code}</Tag>
            </Space>
          ) : (
            <Empty />
          )}
        </Descriptions.Item>
        <Descriptions.Item label="折本币">
          {c ? <Money value={c.amount_base_total} /> : <Empty />}
        </Descriptions.Item>
        <Descriptions.Item label="分期合计">
          {terms.length === 0 ? (
            <Empty />
          ) : (
            <span style={{ color: matched ? undefined : 'var(--ant-color-error)' }}>
              <Money value={termsTotal} />
              {matched ? '' : '（与总额不一致）'}
            </span>
          )}
        </Descriptions.Item>
        <Descriptions.Item label="备注" span={2}>
          {c?.remark || <Empty />}
        </Descriptions.Item>
      </Descriptions>

      <Divider orientation="left" style={{ margin: '24px 0 12px' }}>
        付款分期
      </Divider>

      <ResponsiveTable<ContractTerm>
        rowKey="id"
        size="small"
        rows={terms}
        pagination={false}
        // 第一列是序号，卡上该写期次（定金 / 尾款…）—— 那才是"哪一期"。
        cardTitleKey="name"
        // 分期合计必须等于合同总额（服务端在保存时校验），窄屏也照给：
        // 分几期看不出"一共要付多少"。
        cardSummary={{
          title: '合计',
          facts: [
            {
              key: 'amount',
              label: '金额',
              value: (
                <Space size={4}>
                  <Money value={termsTotal} />
                  <Tag>{c?.currency_code}</Tag>
                </Space>
              ),
            },
            { key: 'base', label: '折本币', value: <Money value={termsBaseTotal} /> },
          ],
        }}
        summary={() =>
          terms.length === 0 ? null : (
            <Table.Summary.Row>
              <Table.Summary.Cell index={0} colSpan={2}>
                合计
              </Table.Summary.Cell>
              <Table.Summary.Cell index={1} align="right">
                <Money value={termsTotal} />
              </Table.Summary.Cell>
              <Table.Summary.Cell index={2} align="right">
                <Money value={termsBaseTotal} />
              </Table.Summary.Cell>
              <Table.Summary.Cell index={3} colSpan={3} />
            </Table.Summary.Row>
          )
        }
      >
        <Table.Column dataIndex="seq" title="#" width={56} />
        <Table.Column dataIndex="name" title="期次" width={180} />
        <Table.Column
          dataIndex="amount"
          title="金额"
          width={150}
          align="right"
          render={(value: number) => (
            <Space size={4}>
              <Money value={value} />
              <Tag>{c?.currency_code}</Tag>
            </Space>
          )}
        />
        <Table.Column
          dataIndex="amount_base"
          title="折本币"
          width={140}
          align="right"
          render={(value: number) => <Money value={value} />}
        />
        {/* 已收 / 未收（T-61）：数来自这一期开出的应收单——核销额在单上，
            贷项在另一张表，服务端算完再给。没开票的那一期按整期未收显示。 */}
        <Table.Column
          title="已收"
          width={130}
          align="right"
          render={(_, row: ContractTerm) => (
            <>
              <Money value={row.received_amount ?? 0} currency={c?.currency_code} />
              {Number(row.credited_amount ?? 0) > 0 && (
                <div>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    冲减 <Money value={row.credited_amount} currency={c?.currency_code} />
                  </Typography.Text>
                </div>
              )}
            </>
          )}
        />
        <Table.Column
          title="未收"
          width={130}
          align="right"
          render={(_, row: ContractTerm) => (
            <Money
              value={row.outstanding_amount ?? row.amount}
              currency={c?.currency_code}
            />
          )}
        />
        <Table.Column
          dataIndex="due_date"
          title="付款日"
          width={120}
          render={(value: string | null) => formatDate(value ?? undefined)}
        />
        <Table.Column
          dataIndex="trigger"
          title="付款条件"
          render={(value: string) => value || <Empty />}
        />
        <Table.Column
          title="开票"
          width={110}
          render={(_, row: ContractTerm) => {
            if (row.receivable_id) return <Tag color="success">已开票</Tag>
            // 草稿与已取消的合同不能开票：货还没确认，开出来没有依据。
            if (c?.status !== 20) return <Empty />
            return (
              <Button size="small" type="link" onClick={() => void invoice(row)}>
                开票
              </Button>
            )
          }}
        />
      </ResponsiveTable>

      {/* 条款是**快照**：这里显示的是生成时抄下来的那几句话，
          改条款库的模板不会改动已经生成的合同。没选条款就不占版面。 */}
      {clauses.length > 0 && (
        <>
          <Divider orientation="left" style={{ margin: '24px 0 12px' }}>
            合同条款
          </Divider>
          <ResponsiveTable<ContractClause>
            rowKey="id"
            size="small"
            rows={clauses}
            pagination={false}
            // 第一列是序号，卡上该写条款标题；正文长，竖着读比横着滑好。
            cardTitleKey="title"
          >
            <Table.Column dataIndex="seq" title="#" width={56} />
            <Table.Column
              dataIndex="title"
              title="条款"
              width={220}
              render={(value: string) => <span className="cell-strong">{value}</span>}
            />
            <Table.Column
              dataIndex="body"
              title="正文"
              render={(value: string) => (
                <Typography.Paragraph style={{ marginBottom: 0, whiteSpace: 'pre-wrap' }}>
                  {value}
                </Typography.Paragraph>
              )}
            />
          </ResponsiveTable>
        </>
      )}

      <div style={{ marginTop: 12, color: 'var(--ant-color-text-tertiary)', fontSize: 12 }}>
        合同不存货物明细：明细留在来源单据上，这里只有一份对外承诺的快照。
        单号 <Mono>{c?.doc_no ?? ''}</Mono> 本身能看出是 PI 还是销售合同。
      </div>
      {/* T-63：后端早就能挂附件，前端入口不全，等于大部分能力用不上。
          客户回签与合同附件挂在这里，比挂在主体上更有用——每家客户一年有几十张单。 */}
      {c?.id ? (
        <AttachmentPanel entityType="contract" entityId={c.id} title="客户回签与合同附件" />
      ) : null}

    </Show>
  )
}
