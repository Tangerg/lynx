// Barrel re-export — keeps every existing `from "./types"` or
// `from "@/plugins/sdk/types"` import working after the per-domain split.
//
// The single 1045-line types.ts has been broken into thirteen domain
// files (common, tool, message, agui, theme, composer, sidebar, commands,
// workspace, infra, host, plugin). Re-export everything here so callers
// pick types from one entry point — domain boundaries matter for authors,
// not consumers.

export * from "./agui";
export * from "./commands";
export * from "./common";
export * from "./composer";
export * from "./host";
export * from "./i18n";
export * from "./infra";
export * from "./message";
export * from "./plugin";
export * from "./sidebar";
export * from "./theme";
export * from "./tool";
export * from "./workspace";
