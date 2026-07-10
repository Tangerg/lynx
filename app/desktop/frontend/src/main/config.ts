// Composition-time configuration constants. Lives in `main/` because it
// belongs to "how this app is wired to the outside world" — not to UI,
// not to plugin runtime, not to a single transport. The composition
// root (container.ts) reads from here; `lib/http` (the plugin-aware
// RPC facade) also reads the default base URL from here so the
// constant has a single owner.
//
// Plugin config (`host.config.set("api.baseUrl", "...")`) can override
// this at runtime; this file just supplies the first-paint default.

/** Default base URL for the local Go Lyra Runtime mock backend. */
export const RUNTIME_BASE = "http://127.0.0.1:17171";

/**
 * Runtime Protocol version this client sends in request metadata. Date
 * string per API.md §11 ("2026-MM-DD"); bumped when the wire contract
 * changes incompatibly. Matches the frozen v2 baseline + the Go runtime's
 * `ProtocolVersion` const.
 */
export const PROTOCOL_VERSION = "2026-06-07";

/** Identifies this client to the runtime in request metadata. */
export const CLIENT_INFO = { name: "lyra-desktop", version: "0.0.0" } as const;
