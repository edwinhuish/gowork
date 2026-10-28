import type { ReactNode } from 'react'

interface FieldProps {
  label: string
  required?: boolean
  className?: string
  children: ReactNode
}

export function Field({ label, required, className, children }: FieldProps) {
  return (
    <div className={className ? `form-item ${className}` : 'form-item'}>
      <label className="form-label">
        {label}
        {required && <span className="req">*</span>}
      </label>
      {children}
    </div>
  )
}
