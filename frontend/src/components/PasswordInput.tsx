import { Button, Input, Space, message } from 'antd'
import { generatePassword } from '@/utils/password'

export function PasswordInput({
  value,
  onChange,
  id
}: {
  value?: string
  onChange?: (value: string) => void
  id?: string
}) {
  return (
    <Space.Compact style={{ width: '100%' }}>
      <Input.Password
        id={id}
        value={value}
        onChange={(event) => onChange?.(event.target.value)}
        autoComplete="new-password"
      />
      <Button
        onClick={() => {
          try {
            onChange?.(generatePassword())
          } catch {
            message.error('当前环境无法安全生成密码，请使用 HTTPS 访问')
          }
        }}
      >
        生成随机密码
      </Button>
      <Button
        disabled={!value}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(value ?? '')
            message.success('密码已复制，请通过安全渠道交付')
          } catch {
            message.error('复制失败，请显示密码后手动复制')
          }
        }}
      >
        复制
      </Button>
    </Space.Compact>
  )
}
