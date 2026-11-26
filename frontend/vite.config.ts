import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '')
  const apiTarget = env.API_PROXY_TARGET || 'http://127.0.0.1:9090'
  const apiProxy = { target: apiTarget }

  return {
    plugins: [vue()],
    server: {
      proxy: {
        '/health': apiProxy,
        '/api': apiProxy,
        '/auth': apiProxy,
        '/admin': apiProxy,
        '/v1': apiProxy,
      },
    },
  }
})
