import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// The build output goes straight into the Go backend's embed directory —
// see backend/internal/webui. `npm run dev` proxies /api to a locally
// running backend (`go run ./cmd/dreamhub`, default port 8080) so the UI
// can be developed without rebuilding the Go binary on every change.
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: false,
      },
    },
  },
  build: {
    outDir: '../backend/internal/webui/dist',
    emptyOutDir: true,
  },
})
