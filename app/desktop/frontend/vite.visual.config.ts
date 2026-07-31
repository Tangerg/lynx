import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

/**
 * Test-only visual fixture entry.
 *
 * It deliberately lives outside the production router and Wails bootstrap:
 * fixtures may freeze clocks, content, appearance, and viewport without adding
 * a debug branch to the shipped application. They still import the production
 * CSS and components, so a screenshot exercises the same visual implementation.
 */
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    host: "127.0.0.1",
    port: 4174,
    strictPort: true,
  },
  build: {
    outDir: "../.cache/visual-dist",
    emptyOutDir: true,
    target: "chrome131",
    // Agent-state fixtures intentionally render the production Markdown,
    // Shiki, and ELK-backed diagram paths. They are lazy from the foundation
    // entry and carry the same reviewed capability shape as production.
    chunkSizeWarningLimit: 1600,
    rollupOptions: {
      input: path.resolve(import.meta.dirname, "visual/index.html"),
    },
  },
});
