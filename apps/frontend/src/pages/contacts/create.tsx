import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Contact } from '@/types'

import { ContactFormFields } from './form-fields'

export function ContactCreate() {
  const { formProps, saveButtonProps } = useForm<Contact>({
    resource: 'contacts',
    action: 'create',
    redirect: 'list',
  })

  return (
    <PageForm action="create" title="新建联系人" formProps={formProps} saveButtonProps={saveButtonProps}>
      <ContactFormFields />
    </PageForm>
  )
}
