import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Dev server proxia /api e /resources para o backend FastAPI (porta 8000)
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://localhost:8000',
      '/resources': 'http://localhost:8000',
    },
  },
  build: { outDir: 'dist' },
})
