import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const apiProxyTarget = process.env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:8000'

export default defineConfig({
  base: './',
  plugins: [react()],
  server: {
    proxy: {
      '/api': apiProxyTarget,
      '/ai': apiProxyTarget,
    },
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('/node_modules/gsap/') || id.includes('/node_modules/@gsap/react/')) {
            return 'gsap'
          }
        },
      },
    },
  },
})
