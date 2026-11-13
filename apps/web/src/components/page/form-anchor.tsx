import { Form } from 'antd'
import type { FormProps } from 'antd'

/**
 * 接住 refine 的 `useTable` 建好的那个表单实例。
 *
 * `useTable` 每次都会 `Form.useForm()` 建一个实例放进 `searchFormProps`，
 * 不管页面用不用筛选。实例如果没被任何 `<Form>` 接上，antd 会在控制台报
 * 「Instance created by useForm is not connected to any Form element」——
 * 开发期噪音，但它会把真正的错误淹掉，而且意味着 `setFilters` 那条链路
 * 少了一环。
 *
 * 所以「有筛选表单」的页面照常用 `<Form {...searchFormProps}>` 渲染；
 * 没有筛选的页面（收付款、应收应付列表，以及详情页里按外键锁死条件的
 * 那几个表格面板）在这里挂一个不可见的 Form。它没有任何交互，
 * 也不会出现在布局里。
 */
export function FormAnchor({ formProps }: { formProps?: FormProps }) {
  if (!formProps?.form) return null
  return (
    <Form {...formProps} style={{ display: 'none' }}>
      {null}
    </Form>
  )
}
