import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vite'

const backend = process.env.BACKEND_URL ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy: {
      '/api': backend,
      '/healthz': backend,
      '/ws': { target: backend, ws: true },
    },
  },
})
