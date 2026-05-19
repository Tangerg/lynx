// Host API version — a single constant in a leaf module.
//
// Why its own file: definePlugin needs HOST_API_VERSION for the semver gate,
// hostBridge needs it for window.__LYRA__. If we kept it on hostBridge,
// definePlugin → hostBridge → @/plugins/sdk → definePlugin is a cycle.
// Leaf module = no cycle, deterministic init order.
export const HOST_API_VERSION = "1.0.0";
