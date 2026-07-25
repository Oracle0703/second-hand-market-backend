import type { FormEvent, ReactNode } from 'react'

type SubmitterConfig = {
  searchConfig?: {
    submitText?: string
  }
}

type FormProps = {
  children?: ReactNode
  onFinish?: (values: Record<string, string>) => boolean | Promise<boolean>
  submitter?: SubmitterConfig | false
}

function TestForm({ children, onFinish, submitter }: FormProps) {
  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const values = Object.fromEntries(new FormData(event.currentTarget).entries()) as Record<string, string>
    await onFinish?.(values)
  }

  return (
    <form onSubmit={handleSubmit}>
      {children}
      {submitter === false ? null : (
        <button type="submit">{submitter?.searchConfig?.submitText ?? '提交'}</button>
      )}
    </form>
  )
}

export function LoginFormPage({ children, onFinish, submitter }: FormProps) {
  return (
    <TestForm onFinish={onFinish} submitter={submitter}>
      {children}
    </TestForm>
  )
}

export const ProForm = TestForm

type TextFieldProps = {
  name: string
  label: string
  fieldProps?: {
    autoComplete?: string
    maxLength?: number
  }
}

function TextField({ name, label, fieldProps }: TextFieldProps) {
  return (
    <label>
      {label}
      <input name={name} {...fieldProps} />
    </label>
  )
}

function PasswordField({ name, label, fieldProps }: TextFieldProps) {
  return (
    <label>
      {label}
      <input name={name} type="password" {...fieldProps} />
    </label>
  )
}

export const ProFormText = Object.assign(TextField, { Password: PasswordField })

export function PageContainer({ children }: { children?: ReactNode }) {
  return <main>{children}</main>
}

export function ProCard({ children }: { children?: ReactNode }) {
  return <section>{children}</section>
}

type ProLayoutProps = {
  children?: ReactNode
  actionsRender?: () => ReactNode
  title?: ReactNode
}

export function ProLayout({ children, actionsRender, title }: ProLayoutProps) {
  return (
    <div data-testid="pro-layout">
      {title ? <div data-testid="pro-layout-title">{title}</div> : null}
      <div data-testid="pro-layout-actions">{actionsRender?.()}</div>
      <div data-testid="pro-layout-content">{children}</div>
    </div>
  )
}
