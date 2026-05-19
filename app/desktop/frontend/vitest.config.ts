/// <reference types="vitest" />
import { defineConfig } from "vitest/config";
import path from "node:path";

// We don't extend vite.config.ts here because that config pulls in the Wails
// runtime + several browser-only plugins; tests run in happy-dom and only
// need the path alias.
export default defineConfig({
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    environment: "happy-dom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
