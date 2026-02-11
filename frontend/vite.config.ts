import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '')
  const proxyTarget = env.VITE_DEV_PROXY_TARGET || 'http://localhost:8080'

  return {
    plugins: [vue()],
    server: {
      proxy: {
        '/api': {
          target: proxyTarget,
          changeOrigin: true
        }
      }
    }
  }
})
