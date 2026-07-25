// The built-in panes' ids — one home for the vocabulary.
//
// A pane's id is how the rest of the app links to it: `openWorkspaceSettingsPane`
// takes one, and four foreign callsites (the sidebar's two shortcuts, the welcome
// screen, the workspace's MCP rows) had been spelling them as literals. Nothing
// checked those against the panes that declare them — rename a pane and the call
// still compiles, the click just lands nowhere. So the id is a symbol both the
// declaration and the link share, the way a data-provider key is.
//
// Third-party panes keep declaring free-form ids (`SettingsPaneSpec.id` is a
// string, and the point is open); this is the built-in set.

export const APPEARANCE_PANE = "appearance";
export const PERSONALIZATION_PANE = "personalization";
export const PROVIDERS_PANE = "providers";
export const APPROVALS_PANE = "approvals";
export const MCP_SERVERS_PANE = "mcp-servers";
export const HOOKS_PANE = "hooks";
export const SCHEDULES_PANE = "schedules";
export const PLUGINS_PANE = "plugins";
export const USAGE_PANE = "usage";
export const CONNECTION_PANE = "connection";
export const BRAND_ICONS_PANE = "brand-icons";
