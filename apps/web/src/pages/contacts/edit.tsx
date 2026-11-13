import { useForm } from '@refinedev/antd'

import { PageForm } from '@/components/page'
import type { Contact } from '@/types'

import { ContactFormFields } from './form-fields'

export function ContactEdit() {
  const { formProps, saveButtonProps } = useForm<Contact>({
    resource: 'contacts',
    action: 'edit',
    redirect: 'list',
  })

  return (
    <PageForm action="edit" title="编辑联系人" formProps={formProps} saveButtonProps={saveButtonProps}>
      <ContactFormFields partnerReadOnly />
    </PageForm>
  )
}
