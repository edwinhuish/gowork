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
        //
        // 分块必须构成单向依赖，否则 Rollup 会报 "Circular chunk"，产物里更难受：
        // 循环里的模块初始化顺序不保证，antd 拿到的是未赋值的 React，
        // 于是浏览器直接抛 "Cannot read properties of undefined (reading 'version')"。
        // 现在的层次是 react <- vendor <- antd <- refine <- index，每一层只往下依赖。
        // 注意 antd 这一档必须把 sunflower-antd 一起收进来：它的包名不含 /antd/，
        // 漏掉就会落进 vendor，而它 import antd、antd 又 import vendor 里的 @babel/runtime，
        // vendor 与 antd 立刻又互为上下游。
        manualChunks(id) {
          if (!id.includes('node_modules')) return
          if (id.includes('@refinedev')) return 'refine'
          if (
            id.includes('/antd/') ||
            id.includes('/rc-') ||
            id.includes('@rc-component') ||
            id.includes('@ant-design') ||
            id.includes('sunflower-antd') ||
            id.includes('/dayjs/')
          ) {
            return 'antd'
          }
          // react 只留最底层的三件，且用精确前缀匹配。
          // 写成 id.includes('/react') 会把 react-router、@tanstack/react-query、
          // react-is、use-sync-external-store 一起吸进来——它们又依赖 vendor 里的包，
          // react 与 vendor 就互为上下游，循环由此产生。这几个包留给 vendor。
          if (/(^|\/)node_modules\/(react|react-dom|scheduler)\//.test(id)) return 'react'
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
