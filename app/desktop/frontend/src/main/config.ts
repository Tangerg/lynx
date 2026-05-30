// Composition-time configuration constants. Lives in `main/` because it
// belongs to "how this app is wired to the outside world" — not to UI,
// not to plugin runtime, not to a single transport. The composition
// root (container.ts) reads from here; `lib/http` (the plugin-aware
// RPC facade) also reads the default base URL from here so the
// constant has a single owner.
//
// Plugin config (`host.config.set("api.baseUrl", "...")`) can override
// this at runtime; this file just supplies the first-paint default.

/** Default base URL for the local Go AG-UI mock backend. */
export const AGUI_BASE = "http://127.0.0.1:17171";

/**
 * Runtime Protocol version this client negotiates at handshake. Date
 * string per API.md §8.2 ("2026-MM-DD"); bumped when the wire contract
 * changes incompatibly.
 */
export const PROTOCOL_VERSION = "2026-05-28";

/** Identifies this client to the runtime at handshake (API.md §6.1). */
export const CLIENT_INFO = { name: "lyra-desktop", version: "0.0.0" } as const;
