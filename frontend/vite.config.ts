import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv } from 'vite'

// The repo has a single .env at its root, shared with backend and infra.
const envDir = path.resolve(__dirname, '..')

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, envDir, '')
  const apiPort = env.APP_PORT || '4700'

  return {
    envDir,
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: { '@': path.resolve(__dirname, './src') },
    },
    server: {
      port: Number(env.FRONTEND_DEV_PORT || 5173),
      strictPort: true,
      // With VITE_USE_MOCKS=false, /api goes to the Go backend.
      proxy: {
        '/api': { target: `http://localhost:${apiPort}`, ws: true },
      },
    },
  }
})
