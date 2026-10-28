import { Edit, useForm } from '@refinedev/antd'
import { Form } from 'antd'

import type { Contact } from '@/types'

import { ContactFormFields } from './form-fields'

export function ContactEdit() {
  const { formProps, saveButtonProps } = useForm<Contact>({
    resource: 'contacts',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <Edit saveButtonProps={saveButtonProps} title="编辑联系人">
      <Form {...formProps} layout="vertical" className="entity-form">
        <ContactFormFields partnerReadOnly />
      </Form>
    </Edit>
  )
}
