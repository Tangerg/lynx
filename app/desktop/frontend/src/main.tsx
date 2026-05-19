import { createRoot } from "react-dom/client";
import "./style.css";
import App from "./App";

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
