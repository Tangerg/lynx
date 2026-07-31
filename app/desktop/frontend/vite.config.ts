import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { readFileSync } from "node:fs";
import path from "node:path";

// Where the dev server lives is declared once, in the shell manifest Wails reads
// at startup (`frontend:dev:serverUrl`) — the file a human edits to move it. This
// derives from that instead of restating the number: the port used to be spelled
// four times across two languages, and the host disagreed outright (`localhost`
// there, `127.0.0.1` here), which resolves to ::1 on some machines and leaves the
// WebView pointed at a port nothing is listening on.
const devServer = new URL(
  (
    JSON.parse(readFileSync(path.resolve(import.meta.dirname, "../wails.json"), "utf8")) as {
      "frontend:dev:serverUrl": string;
    }
  )["frontend:dev:serverUrl"],
);

export default defineConfig({
  plugins: [react(), tailwindcss()],
  // Wails serves the webview through its OWN dev server (a different port than
  // Vite), so the page origin is NOT Vite's port. Without `hmr.clientPort` the
  // Vite HMR client in the WebView would open its WebSocket against the page
  // origin (the Wails dev-server port) instead of Vite — the handshake fails
  // silently and updates compile but never reach the window. Pinning the port
  // (strict, no fallback drift) + clientPort makes the HMR socket deterministic.
  server: {
    host: devServer.hostname,
    port: Number(devServer.port),
    strictPort: true,
    hmr: { protocol: "ws", host: devServer.hostname, clientPort: Number(devServer.port) },
  },
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  build: {
    // Desktop app loads from disk, so chunk size is less critical than on web.
    // Still, splitting vendor deps means Wails updates only ship changed chunks.
    // The largest intentional lazy features are ELK-backed diagrams and Shiki;
    // their compressed payloads have explicit budgets in check-bundle-size.mjs.
    // Keep Vite's raw warning just above those known monoliths so it catches a
    // new unclassified mega-chunk without reporting the same reviewed features
    // on every clean build.
    chunkSizeWarningLimit: 1600,
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
          // Headless interaction primitives
          if (id.includes("node_modules/@base-ui")) return "base-ui";
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
          // Leave unrelated dependencies to Rollup's graph-aware chunking. A
          // catch-all vendor bucket merged otherwise independent lazy features
          // into one 9MB raw chunk and defeated the explicit boundaries above.
          return undefined;
        },
      },
    },
    target: "chrome131",
  },
});
