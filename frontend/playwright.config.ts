import { defineConfig, devices } from '@playwright/test'

/**
 * Integration tests require the full stack running.
 * Set BASE_URL to point to the app (default: http://localhost:5173).
 *
 * Quick start:
 *   npm run dev          # in one terminal
 *   npm run test:e2e     # in another
 */
export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  retries: 0,
  use: {
    baseURL: process.env.BASE_URL ?? 'http://localhost:5173',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
