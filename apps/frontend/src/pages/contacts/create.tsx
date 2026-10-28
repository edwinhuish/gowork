import { Create, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Contact } from '@/types'

import { ContactFormFields } from './form-fields'

export function ContactCreate() {
  const { formProps, saveButtonProps } = useForm<Contact>({
    resource: 'contacts',
    action: 'create',
    redirect: 'list',
  })

  return (
    <Create saveButtonProps={saveButtonProps} title="新建联系人" goBack={null}>
      <Form {...formProps} layout="vertical" className="entity-form">
        <ContactFormFields />
      </Form>
    </Create>
  )
}
