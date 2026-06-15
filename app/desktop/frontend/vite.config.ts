import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    // Desktop app loads from disk, so chunk size is less critical than on web.
    // Still, splitting vendor deps means Wails updates only ship changed chunks.
    rollupOptions: {
      output: {
        manualChunks(id: string) {
          // Stable vendor deps
          if (
            id.includes("node_modules/react") ||
            id.includes("node_modules/react-dom") ||
            id.includes("node_modules/scheduler")
          )
            return "vendor";
          if (id.includes("node_modules/motion")) return "vendor-motion";
          if (id.includes("node_modules/zustand")) return "vendor";
          // Radix UI
          if (id.includes("node_modules/@radix-ui")) return "radix";
          // TanStack
          if (id.includes("node_modules/@tanstack")) return "tanstack";
          // Icons
          if (id.includes("node_modules/@lobehub/icons")) return "icons";
          if (id.includes("node_modules/lucide-react")) return "icons";
          // Markdown + syntax highlighting
          if (
            id.includes("node_modules/react-markdown") ||
            id.includes("node_modules/remark-") ||
            id.includes("node_modules/rehype-") ||
            id.includes("node_modules/unist-") ||
            id.includes("node_modules/mdast-") ||
            id.includes("node_modules/shiki")
          )
            return "markdown";
          // Math rendering
          if (id.includes("node_modules/katex") || id.includes("node_modules/remark-math"))
            return "katex";
          // Mermaid
          if (id.includes("node_modules/beautiful-mermaid")) return "mermaid";
          // OpenTelemetry — only used in diagnostics view
          if (id.includes("node_modules/@opentelemetry")) return "otel";
          // Other node_modules
          if (id.includes("node_modules")) return "vendor-libs";
          return undefined;
        },
      },
    },
    target: "chrome131",
  },
});
