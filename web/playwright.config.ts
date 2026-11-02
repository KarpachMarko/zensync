import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  workers: 1,
  fullyParallel: false,
  use: { baseURL: "http://127.0.0.1:18081", headless: true },
  webServer: {
    command: "sh ../scripts/test-server.sh",
    url: "http://127.0.0.1:18081/readyz",
    reuseExistingServer: false,
    timeout: 120000,
  },
  reporter: "list",
});
