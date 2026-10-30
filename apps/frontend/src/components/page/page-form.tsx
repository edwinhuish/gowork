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
  /** refine useForm 返回的 saveButtonProps（含 disabled/onClick）。 */
  saveButtonProps?: object
  /** 表头额外动作，排在保存按钮左侧。 */
  extra?: ReactNode
  /** 表单上方的提示条（如"这张单已锁定，不能再改"），在 Form 之外。 */
  notice?: ReactNode
  /** 详情数据加载中：refine 外壳的卡片骨架。 */
  loading?: boolean
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
  notice,
  loading,
  wide,
  children,
}: PageFormProps) {
  const Shell = action === 'create' ? Create : Edit
  // 保存按钮统一带一个对勾图标：refine 的默认按钮只有文字，单据页上的「提交」
  // 与「打印」挨在一起时不好一眼分辨。
  const saveProps = { icon: <CheckCircleOutlined />, ...(saveButtonProps ?? {}) } as never

  return (
    <Shell
      title={title}
      goBack={null}
      isLoading={loading}
      saveButtonProps={saveProps}
      headerButtons={extra ? <Space wrap>{extra}</Space> : undefined}
    >
      {notice}
      <Form
        {...formProps}
        layout="vertical"
        className={wide ? 'entity-form entity-form--wide' : 'entity-form'}
        /*
         * 校验失败要滚到第一条错误：窄屏表单是竖排的，一屏放不下，
         * 提交按钮最靠下 —— 不滚的话错误留在屏幕上方，用户点了「保存」
         * 什么都看不到，只当是没反应（客户 / 合同新建页实测：错误提示
         * 确实生成了，但一条都不在视口里）。
         */
        scrollToFirstError={{ behavior: 'smooth', block: 'center' }}
      >
        {children}
      </Form>
    </Shell>
  )
}
