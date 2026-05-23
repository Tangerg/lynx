// Barrel re-export — keeps every existing `from "./types"` or
// `from "@/plugins/sdk/types"` import working after the per-domain split.
//
// The single 1045-line types.ts has been broken into thirteen domain
// files (common, tool, message, agui, theme, composer, sidebar, commands,
// workspace, infra, host, plugin). Re-export everything here so callers
// pick types from one entry point — domain boundaries matter for authors,
// not consumers.

export * from "./common";
export * from "./tool";
export * from "./message";
export * from "./agui";
export * from "./theme";
export * from "./composer";
export * from "./sidebar";
export * from "./commands";
export * from "./workspace";
export * from "./infra";
export * from "./host";
export * from "./plugin";
