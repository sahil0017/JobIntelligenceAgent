import { defineConfig, devices } from "@playwright/test";

const apiPort = 18787;
const webPort = 15173;

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  timeout: 30_000,
  expect: { timeout: 7_500 },
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: `http://127.0.0.1:${webPort}`,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  // Both servers run against a throwaway database so tests never touch real data.
  webServer: [
    {
      command: "rm -rf .e2e-data && go run ./platform/cmd/careerpilot",
      url: `http://127.0.0.1:${apiPort}/api/health`,
      env: { API_PORT: String(apiPort), CAREERPILOT_DATA_DIR: ".e2e-data" },
      timeout: 180_000,
      reuseExistingServer: false,
    },
    {
      command: `npm --prefix console run dev -- --host 127.0.0.1 --port ${webPort} --strictPort`,
      url: `http://127.0.0.1:${webPort}`,
      env: { API_PORT: String(apiPort) },
      timeout: 60_000,
      reuseExistingServer: false,
    },
  ],
});
