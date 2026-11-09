import { defineConfig, devices } from "@playwright/test";

// An isolated presentation suite: no Docker, real credentials, or paid providers.
const baseURL = "http://127.0.0.1:3107";

export default defineConfig({
  testDir: "./tests/ui",
  testMatch: "*.spec.ts",
  fullyParallel: true,
  reporter: "list",
  use: { baseURL, trace: "off", screenshot: "only-on-failure" },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
    { name: "mobile", use: { ...devices["iPhone 13"], defaultBrowserType: "chromium" } },
  ],
  webServer: [
    { command: "npx tsx tests/ui/mock-api.ts", url: "http://127.0.0.1:3108/healthz" },
    {
      command: "npm run dev -- --port 3107",
      url: `${baseURL}/login`,
      timeout: 120000,
      env: {
        SG1_ENV: "development",
        SG1_DEV_TOKEN: "theme-test-only-not-a-real-secret-00000000",
        SG1_API_URL: "http://127.0.0.1:3108",
        SG1_PUBLIC_URL: baseURL,
      },
    },
  ],
});
