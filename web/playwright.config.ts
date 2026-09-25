import { defineConfig, devices } from "@playwright/test";

// Roda contra um servidor já de pé (ver docs/DESENVOLVIMENTO.md e o job e2e do CI).
// A origem precisa ser a mesma de PUBLIC_URL: passkeys e a checagem de Origin dependem disso.
export default defineConfig({
  testDir: "e2e",
  timeout: 60_000,
  fullyParallel: false,
  workers: 1,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: process.env.BASE_URL ?? "http://localhost:3100",
    locale: "pt-BR",
    timezoneId: "America/Sao_Paulo",
    trace: "retain-on-failure",
    launchOptions: process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {},
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
