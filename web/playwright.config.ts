import { defineConfig, devices } from "@playwright/test";

/**
 * End-to-end config for the FastProxy admin SPA.
 *
 * Tests run against a *real* backend binary (embedded `admin/dist` build):
 * CI starts it before the suite and passes BASE_URL + ADMIN_PASSWORD, e.g.
 *
 *   CONFIG=/tmp/fp-e2e/fastproxy.db ADMIN_ADDR=127.0.0.1:7788 \
 *   ADMIN_PASSWORD=e2e-admin-pass PORT_RANGE=10001-10050 /tmp/fastproxy &
 *   BASE_URL=http://127.0.0.1:7788 ADMIN_PASSWORD=e2e-admin-pass npx playwright test
 *
 * Locally, build the UI, start the Go binary the same way, then `npm run e2e`.
 * There is no webServer block on purpose: the suite must exercise the real
 * Go server (cookie auth, embedded assets, /api/* handlers), not vite preview.
 */
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: "list",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: process.env.BASE_URL ?? "http://127.0.0.1:7788",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "off",
    locale: "zh-CN",
    viewport: { width: 1280, height: 900 },
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
