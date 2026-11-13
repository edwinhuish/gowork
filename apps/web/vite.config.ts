import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发期把 /api 代理到本地后端，前端代码里统一用相对路径，
// 部署时由网关或反向代理接管，不需要改代码。
export default defineConfig({
  plugins: [react()],
  resolve: {
    // 与 tsconfig.json 的 compilerOptions.paths 保持一致：@/* -> src/*
    //
    // 两处都必须写，缺一不可：tsc 只读 tsconfig 的 paths 做类型解析，
    // vite/rollup 完全不读它，只认这里的 alias。少一边的表现是
    // 「tsc 通过、vite build 报 Could not resolve」——很容易误判成依赖问题。
    //
    // 用 fileURLToPath 而不是 __dirname：package.json 是 type: module，
    // 配置文件按 ESM 加载，__dirname 不存在；也不用 path.resolve 拼字符串，
    // 这样 Windows 下盘符路径不会出现 /C:/ 这种畸形前缀。
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    // 引入 antd + refine 后单包体积接近 1.6MB，全塞进一个 chunk 会让首屏
    // 必须等整个 vendor 下载完。按依赖族拆开：三者版本不同步，分开缓存后
    // 升级其中一族不会让另外两族的缓存一起失效。
    rollupOptions: {
      output: {
        // 用函数按来源归类，而不是列包名数组：antd 的依赖大量来自 rc-* 系列，
        // 数组写法只能命中显式列出的包，rc-* 会被算进业务 chunk（实测混进 244KB）。
        manualChunks(id) {
          if (!id.includes('node_modules')) return
          if (id.includes('@refinedev')) return 'refine'
          if (
            id.includes('/antd/') ||
            id.includes('/rc-') ||
            id.includes('@ant-design') ||
            id.includes('/dayjs/')
          ) {
            return 'antd'
          }
          if (id.includes('/react') || id.includes('/scheduler/')) return 'react'
          return 'vendor'
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
})
