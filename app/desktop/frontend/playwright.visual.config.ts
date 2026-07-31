import { defineConfig } from "@playwright/test";

const VISUAL_PORT = 4174;

export default defineConfig({
  testDir: "./visual",
  testMatch: "**/*.visual.spec.ts",
  fullyParallel: true,
  forbidOnly: true,
  retries: 0,
  workers: 2,
  reporter: "line",
  outputDir: "../.cache/playwright-results",
  use: {
    baseURL: `http://127.0.0.1:${VISUAL_PORT}`,
    browserName: "chromium",
    colorScheme: "light",
    deviceScaleFactor: 1,
    locale: "en-US",
    reducedMotion: "reduce",
    timezoneId: "UTC",
    viewport: { width: 1120, height: 720 },
  },
  expect: {
    toHaveScreenshot: {
      animations: "disabled",
      caret: "hide",
      scale: "css",
    },
  },
  webServer: {
    command: "npm run visual:dev",
    url: `http://127.0.0.1:${VISUAL_PORT}/visual/`,
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
