import { LockOutlined, LoginOutlined, MoonOutlined, SunOutlined, UserOutlined } from '@ant-design/icons'
import { useLogin } from '@refinedev/core'
import { Alert, Button, Card, Flex, Form, Input, Tooltip, Typography } from 'antd'

import { useUiPrefs } from '@/providers/uiPrefs'

interface LoginForm {
  username: string
  password: string
}

/**
 * 登录页。
 *
 * 走 refine 的 useLogin：登录成功后由 authProvider.login 写入令牌与身份缓存，
 * 再由 refine 负责跳转，页面本身不需要碰 location 和 token。
 *
 * 明暗切换放在右上角：登录页在工作台外壳之外，没有顶栏那个开关，
 * 深色偏好的用户在这里进不来就只能先被闪一下白。
 */
export function LoginPage() {
  const { mutate: login, isPending, error } = useLogin<LoginForm>()
  const { mode, toggleMode } = useUiPrefs()

  return (
    <div className="login-page">
      <div className="login-theme">
        <Tooltip title={mode === 'dark' ? '切换到浅色' : '切换到深色'}>
          <Button
            type="text"
            shape="circle"
            aria-label="切换主题"
            icon={mode === 'dark' ? <SunOutlined /> : <MoonOutlined />}
            onClick={toggleMode}
          />
        </Tooltip>
      </div>

      <Card className="login-card" variant="borderless" styles={{ body: { padding: 28 } }}>
        <Flex vertical gap={20}>
          <div className="login-brand">
            <span className="brand-mark">G</span>
            <div>
              <Typography.Title level={4} style={{ margin: 0 }}>
                GoWork
              </Typography.Title>
              <Typography.Text type="secondary">外贸 ERP · CRM 一体化平台</Typography.Text>
            </div>
          </div>

          {error && (
            <Alert type="error" showIcon message={(error as Error).message || '登录失败'} />
          )}

          <Form<LoginForm>
            layout="vertical"
            requiredMark={false}
            onFinish={(values) => login(values)}
            // 预填演示账号，方便本地首次启动直接进入；生产环境请改掉默认密码。
            initialValues={{ username: 'admin', password: 'admin123' }}
          >
            <Form.Item
              name="username"
              label="用户名"
              rules={[{ required: true, message: '请填写用户名' }]}
            >
              <Input prefix={<UserOutlined />} autoComplete="username" size="large" />
            </Form.Item>

            <Form.Item
              name="password"
              label="密码"
              rules={[{ required: true, message: '请填写密码' }]}
            >
              <Input.Password
                prefix={<LockOutlined />}
                autoComplete="current-password"
                size="large"
              />
            </Form.Item>

            <Button
              type="primary"
              htmlType="submit"
              size="large"
              block
              icon={<LoginOutlined />}
              loading={isPending}
            >
              登录
            </Button>
          </Form>

          <Typography.Text type="secondary" className="login-hint">
            首次启动默认账号 admin / admin123
          </Typography.Text>
        </Flex>
      </Card>
    </div>
  )
}
