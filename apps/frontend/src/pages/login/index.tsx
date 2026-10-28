import { useLogin } from '@refinedev/core'
import { Alert, Button, Card, Form, Input, Typography } from 'antd'

interface LoginForm {
  username: string
  password: string
}

/**
 * 登录页。
 *
 * 走 refine 的 useLogin：登录成功后由 authProvider.login 写入令牌与身份缓存，
 * 再由 refine 负责跳转，页面本身不需要碰 location 和 token。
 */
export function LoginPage() {
  const { mutate: login, isPending, error } = useLogin<LoginForm>()

  return (
    <div className="login-page">
      <Card className="login-card" variant="borderless">
        <div className="login-brand">
          <span className="brand-mark">G</span>
          <div>
            <Typography.Title level={4} style={{ margin: 0 }}>
              GoWork ERP
            </Typography.Title>
            <Typography.Text type="secondary">外贸采购与出口管理平台</Typography.Text>
          </div>
        </div>

        {error && (
          <Alert
            type="error"
            showIcon
            style={{ marginBottom: 16 }}
            message={(error as Error).message || '登录失败'}
          />
        )}

        <Form<LoginForm>
          layout="vertical"
          requiredMark={false}
          onFinish={(values) => login(values)}
          // 预填演示账号，方便本地首次启动直接进入；生产环境请改掉默认密码。
          initialValues={{ username: 'admin', password: 'admin123' }}
        >
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: '请填写用户名' }]}>
            <Input autoComplete="username" size="large" />
          </Form.Item>

          <Form.Item name="password" label="密码" rules={[{ required: true, message: '请填写密码' }]}>
            <Input.Password autoComplete="current-password" size="large" />
          </Form.Item>

          <Button type="primary" htmlType="submit" size="large" block loading={isPending}>
            登录
          </Button>
        </Form>

        <div className="login-hint">首次启动默认账号 admin / admin123</div>
      </Card>
    </div>
  )
}
