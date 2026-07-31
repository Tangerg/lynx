// Composition-time identity for the local desktop process. The selectable
// Runtime Protocol endpoint is owned by the Runtime bounded context; it merely
// happens to use the same URL as this shell in the default development setup.

/** Fixed base URL for assets and metadata served by the local desktop shell. */
export const LOCAL_DESKTOP_SHELL_BASE_URL = "http://127.0.0.1:17171";

/** Identifies this client to the runtime in request metadata. */
export const DESKTOP_CLIENT_INFO = { name: "lyra-desktop", version: "0.0.0" } as const;
