import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  base: './',
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://backend:8000',
      '/ai': 'http://backend:8000',
    },
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
  },
})
