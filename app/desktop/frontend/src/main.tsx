import { createRoot } from "react-dom/client";
import App from "./App";
import { disposeContainer, initializeDesktopHost } from "./main/container";
import { DesktopRenderer } from "./main/renderer";
import { applyWindowChrome, watchWindowChrome } from "./main/windowChrome";
import { disposeOnHmr } from "./lib/hmr";
// Fonts: the native OS stack (SF Pro / PingFang on macOS) — see globals.css
// --font-sans. No bundled webfont; the system face is the premium, native
// default, loads instantly, and renders mixed CJK best.
import "./styles/globals.css";

// NOTE: deliberately not wrapped in StrictMode.
//
// StrictMode double-invokes effects in dev. With our stack (Zustand persist
// rehydrate + AbstractAgent subscribe + plugin loader sequencing), the
// double-invoke surfaces benign-but-confusing "Maximum update depth" warnings
// from React's safety net. The bundle ships without StrictMode in production
// regardless, so removing it here matches what real users see.
//
// Re-enable when we're ready to harden the effect lifecycle (idempotent
// agent subscribe, ref-counted plugin loader, etc.) for true double-invoke
// safety.

const renderer = new DesktopRenderer({
  initializeDesktopHost,
  prepareWindowChrome: applyWindowChrome,
  watchWindowChrome,
  mount() {
    const container = document.getElementById("root");
    const root = createRoot(container!);
    root.render(<App />);
    return root;
  },
  closeRuntime: disposeContainer,
  reportFailure(scope, error) {
    console.error(`[desktop] ${scope} failed:`, error);
  },
});

const teardown = () => {
  void renderer.dispose().catch((error: unknown) => {
    console.error("[desktop] teardown failed:", error);
  });
};
window.addEventListener("beforeunload", teardown);
disposeOnHmr(() => window.removeEventListener("beforeunload", teardown));

void renderer.start().catch((error: unknown) => {
  console.error("[desktop] startup failed:", error);
});
