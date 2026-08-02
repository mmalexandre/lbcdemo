/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  // Expose VITE_API_URL at build time so the production bundle can reach the
  // Go API on EKS through the ALB (different origin from CloudFront).
  // Falls back to '' in dev so the proxy rules below continue to work.
  define: {
    __API_URL__: JSON.stringify(process.env.VITE_API_URL ?? ''),
  },
  plugins: [react()],
  server: {
    proxy: {
      '/login': { target: 'http://localhost:8080', changeOrigin: true },
      '/logout': { target: 'http://localhost:8080', changeOrigin: true },
      '/me': { target: 'http://localhost:8080', changeOrigin: true },
      '/prompt': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
})
