import { CheckCircleOutlined } from '@ant-design/icons'
import { Create, Edit } from '@refinedev/antd'
import { Form, Space } from 'antd'
import type { FormProps } from 'antd'
import type { ReactNode } from 'react'

/**
 * 表单页外壳（新建 / 编辑）。
 *
 * 改造前 39 处 `<Create saveButtonProps><Form layout="vertical" className="entity-form">`，
 * 每一处都要记得 `goBack={null}`、`layout="vertical"`、class 名，漏一个就与其它页不同。
 * 收进来之后新增表单只需要写字段。
 *
 * `wide` 给带明细编辑器的单据表单：两列网格在 880px 里放明细行会横向挤成一团。
 */
export interface PageFormProps {
  action: 'create' | 'edit'
  title?: ReactNode
  formProps: FormProps
  saveButtonProps?: { disabled?: boolean } & Record<string, unknown>
  /** 表头额外动作，排在保存按钮左侧。 */
  extra?: ReactNode
  /** 内容更宽（单据明细类表单）。 */
  wide?: boolean
  children: ReactNode
}

export function PageForm({
  action,
  title,
  formProps,
  saveButtonProps,
  extra,
  wide,
  children,
}: PageFormProps) {
  const Shell = action === 'create' ? Create : Edit
  // 保存按钮统一带一个对勾图标：refine 的默认按钮只有文字，单据页上的「提交」
  // 与「打印」挨在一起时不好一眼分辨。
  const saveProps = { icon: <CheckCircleOutlined />, ...saveButtonProps } as never

  return (
    <Shell
      title={title}
      goBack={null}
      saveButtonProps={saveProps}
      headerButtons={extra ? <Space wrap>{extra}</Space> : undefined}
    >
      <Form
        {...formProps}
        layout="vertical"
        className={wide ? 'entity-form entity-form--wide' : 'entity-form'}
      >
        {children}
      </Form>
    </Shell>
  )
}
