import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

// antd 的 reset 必须先于项目样式加载，否则会被它自己的 normalize 覆盖掉。
import 'antd/dist/reset.css'
import App from './App'
import './index.css'
import { UiPrefsProvider } from './providers/uiPrefs'

const container = document.getElementById('root')
if (!container) {
  throw new Error('找不到 #root 挂载点')
}

// 界面偏好必须在 App 外层：App 里的 ConfigProvider 要用它算主题。
createRoot(container).render(
  <StrictMode>
    <UiPrefsProvider>
      <App />
    </UiPrefsProvider>
  </StrictMode>,
)
