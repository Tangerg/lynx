import { createRoot } from "react-dom/client";
import App from "./App";
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

const container = document.getElementById("root");
const root = createRoot(container!);
root.render(<App />);
