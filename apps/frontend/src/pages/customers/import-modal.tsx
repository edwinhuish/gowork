import { useState } from 'react'
import { Alert, Button, Modal, Space, Table, Tag, Typography, Upload, message } from 'antd'

import { importApi } from '@/api/exports'
import { Mono } from '@/components/fields'
import type { PartnerImportFailure, PartnerImportReport } from '@/types'

/**
 * 主体导入（T-35）。
 *
 * 两步：先 dry_run 出报告，看清哪几行会被跳过，再确认落库。
 * 一份几百行的文件里错两三行是常态——直接导进去之后"哪几行没进来"没人说得清。
 *
 * 模板就是 `/exports/partners` 导出的那份 CSV：表头一致，改完另存即可。
 */
export function PartnerImportModal({
  open,
  onClose,
  onImported,
}: {
  open: boolean
  onClose: () => void
  onImported: () => void | Promise<void>
}) {
  const [file, setFile] = useState<File | null>(null)
  const [report, setReport] = useState<PartnerImportReport | null>(null)
  const [busy, setBusy] = useState(false)

  const reset = () => {
    setFile(null)
    setReport(null)
  }

  const run = async (dryRun: boolean) => {
    if (!file) {
      message.warning('请先选择 CSV 文件')
      return
    }
    setBusy(true)
    try {
      const res = await importApi.partners(file, dryRun)
      setReport(res)
      if (!dryRun) {
        message.success(`已导入 ${res.created} 个主体`)
        await onImported()
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : '导入失败')
    } finally {
      setBusy(false)
    }
  }

  // 失败行下载：让人拿着原文件对着行号改，而不是在弹窗里一行行抄。
  const downloadFailures = () => {
    if (!report?.failures.length) return
    const header = '行号,编码,名称,原因'
    const lines = report.failures.map(
      (f) => `${f.line},${f.code},"${f.name.replace(/"/g, '""')}","${f.reason.replace(/"/g, '""')}"`,
    )
    // 加 BOM：与后端导出一致，Excel 打开中文不乱码。
    const blob = new Blob([`\ufeff${[header, ...lines].join('\n')}`], {
      type: 'text/csv;charset=utf-8',
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'import-failures.csv'
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <Modal
      open={open}
      title="导入主体"
      width={820}
      onCancel={() => {
        reset()
        onClose()
      }}
      destroyOnHidden
      footer={
        <Space>
          <Button
            onClick={() => {
              reset()
              onClose()
            }}
          >
            关闭
          </Button>
          {report?.failures.length ? (
            <Button onClick={downloadFailures}>下载失败行</Button>
          ) : null}
          <Button loading={busy} onClick={() => void run(true)}>
            校验
          </Button>
          <Button
            type="primary"
            loading={busy}
            disabled={!report || report.dry_run === false}
            onClick={() => void run(false)}
          >
            确认导入
          </Button>
        </Space>
      }
    >
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <Typography.Text type="secondary">
          文件是 CSV（Excel 里另存为「CSV UTF-8」）。<b>建议先导出一次，用导出的文件当模板</b>
          ：表头一致，编码 / 名称 / 角色是最关键的列，角色可填 customer / supplier / forwarder
          或 客户 / 供应商 / 货代。
        </Typography.Text>

        <Upload
          accept=".csv"
          maxCount={1}
          beforeUpload={(f) => {
            setFile(f)
            setReport(null)
            return false // 不自动上传：先校验再落库
          }}
          onRemove={() => reset()}
        >
          <Button>选择 CSV 文件</Button>
        </Upload>

        {report && (
          <Alert
            type={report.failures.length > 0 ? 'warning' : 'success'}
            showIcon
            message={
              report.dry_run
                ? `校验完成：共 ${report.total} 行，可导入 ${report.skipped} 行，失败 ${report.failures.length} 行`
                : `导入完成：新建 ${report.created} 个主体，失败 ${report.failures.length} 行`
            }
            description={
              report.duplicates.length > 0
                ? `${report.duplicates.length} 行与库里已有主体相似（不阻断导入，请自行确认是否重复）`
                : undefined
            }
          />
        )}

        {report && report.failures.length > 0 && (
          <Table<PartnerImportFailure>
            size="small"
            rowKey="line"
            dataSource={report.failures}
            pagination={false}
            scroll={{ y: 240 }}
          >
            <Table.Column dataIndex="line" title="行号" width={70} />
            <Table.Column
              dataIndex="code"
              title="编码"
              width={130}
              render={(v: string) => (v ? <Mono>{v}</Mono> : '—')}
            />
            <Table.Column dataIndex="reason" title="原因" />
          </Table>
        )}

        {report && report.duplicates.length > 0 && (
          <Space direction="vertical" size={4} style={{ width: '100%' }}>
            <Typography.Text type="secondary">可能重复（仅提示）：</Typography.Text>
            {report.duplicates.slice(0, 8).map((d) => (
              <span key={d.line}>
                <Tag>第 {d.line} 行</Tag>
                <Mono>{d.code}</Mono> {d.name} —— {d.reasons.join('；')}
              </span>
            ))}
          </Space>
        )}
      </Space>
    </Modal>
  )
}
