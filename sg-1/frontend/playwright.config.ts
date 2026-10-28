import { defineConfig, devices } from "@playwright/test";

const baseURL = process.env.SG1_PUBLIC_URL || "http://localhost:3000";
if (!["localhost", "127.0.0.1"].includes(new URL(baseURL).hostname))
  throw new Error("These development tests must only target a local server");

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  retries: 0,
  reporter: "list",
  use: { baseURL, trace: "off", screenshot: "only-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
