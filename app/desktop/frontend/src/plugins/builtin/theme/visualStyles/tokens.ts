type VisualStyleTokenName =
  | "style-shape-2xs"
  | "style-shape-xs"
  | "style-shape-sm"
  | "style-shape-md"
  | "style-shape-lg"
  | "style-shape-xl"
  | "style-shape-content"
  | "style-shape-composer"
  | "style-shape-bubble"
  | "button-radius"
  | "field-radius"
  | "segmented-radius"
  | "segment-radius"
  | "surface-card-radius"
  | "floating-panel-radius"
  | "floating-tip-radius"
  | "dock-tab-radius"
  | "control-height-xs"
  | "control-height-sm"
  | "control-height-md"
  | "control-height-lg"
  | "field-height-sm"
  | "field-height-md"
  | "field-height-lg"
  | "menu-row-height"
  | "dock-tab-height"
  | "surface-header-height"
  | "control-edge-width"
  | "composer-edge-width"
  | "app-drawer-surface"
  | "app-drawer-backdrop"
  | "app-drawer-shadow"
  | "app-drawer-sheen-opacity"
  | "app-content-surface"
  | "app-content-shadow"
  | "app-content-ring"
  | "app-content-ring-opacity"
  | "app-header-surface"
  | "app-dock-surface"
  | "app-card-surface"
  | "app-surface-divider"
  | "seam-line"
  | "seam-shadow-color"
  | "shadow-border"
  | "shadow-control"
  | "shadow-composer-depth"
  | "shadow-popover"
  | "shadow-well"
  | "shadow-raised-chip"
  | "shadow-surface-card";

export type VisualStyleTokens = Record<VisualStyleTokenName, string>;

export const SYNARA_TOKENS: VisualStyleTokens = {
  "style-shape-2xs": "4px",
  "style-shape-xs": "6px",
  "style-shape-sm": "8px",
  "style-shape-md": "10px",
  "style-shape-lg": "14px",
  "style-shape-xl": "18px",
  "style-shape-content": "14.4px",
  "style-shape-composer": "19.2px",
  "style-shape-bubble": "12.8px",
  "button-radius": "var(--shape-md)",
  "field-radius": "var(--shape-md)",
  "segmented-radius": "var(--shape-md)",
  "segment-radius": "var(--shape-xs)",
  "surface-card-radius": "var(--shape-lg)",
  "floating-panel-radius": "var(--shape-xl)",
  "floating-tip-radius": "var(--shape-md)",
  "dock-tab-radius": "var(--shape-md)",
  "control-height-xs": "24px",
  "control-height-sm": "28px",
  "control-height-md": "32px",
  "control-height-lg": "40px",
  "field-height-sm": "26px",
  "field-height-md": "32px",
  "field-height-lg": "36px",
  "menu-row-height": "26px",
  "dock-tab-height": "28px",
  "surface-header-height": "46px",
  "control-edge-width": "0.5px",
  "composer-edge-width": "1px",
  "app-drawer-surface": "color-mix(in srgb, var(--color-bg) 68%, transparent)",
  "app-drawer-backdrop": "blur(8px) saturate(135%)",
  "app-drawer-shadow": "inset 0 1px 0 color-mix(in srgb, var(--color-text) 2.5%, transparent)",
  "app-drawer-sheen-opacity": "1",
  "app-content-surface": "var(--color-bg)",
  "app-content-shadow": "-6.5px 0 12px -10px var(--seam-shadow-color)",
  "app-content-ring": "inset 0 0 0 1px var(--seam-line)",
  "app-content-ring-opacity": "1",
  "app-header-surface": "transparent",
  "app-dock-surface": "var(--color-bg)",
  "app-card-surface": "var(--color-surface)",
  "app-surface-divider": "color-mix(in srgb, var(--color-border) 60%, transparent)",
  "seam-line": "color-mix(in srgb, var(--color-text) 8%, transparent)",
  "seam-shadow-color": "color-mix(in srgb, var(--color-text) 12%, transparent)",
  "shadow-border": "0 0 0 0.5px color-mix(in srgb, var(--color-text) 9%, transparent)",
  "shadow-control":
    "0 0 0 0.5px color-mix(in srgb, var(--color-text) 9%, transparent), 0 1px 1px color-mix(in srgb, var(--color-text) 4%, transparent), 0 4px 10px -4px color-mix(in srgb, var(--color-text) 7%, transparent)",
  "shadow-composer-depth": "0 6px 30px -8px color-mix(in srgb, var(--color-text) 9%, transparent)",
  "shadow-popover":
    "0 0 0 1px var(--seam-line), 0 10px 30px -10px color-mix(in srgb, var(--color-text) 14%, transparent)",
  "shadow-well": "inset 0 1px 2px color-mix(in srgb, var(--color-text) 6%, transparent)",
  "shadow-raised-chip":
    "0 1px 1.5px color-mix(in srgb, var(--color-text) 6%, transparent), inset 0 1px 0 color-mix(in srgb, white 45%, transparent)",
  "shadow-surface-card": "none",
};

export const SYNARA_MOTION = {
  instantMs: 80,
  fastMs: 150,
  mediumMs: 200,
  disclosureMs: 220,
  slowMs: 360,
  drawerMs: 300,
  easeOut: [0.22, 1, 0.36, 1],
  easeInOut: [0.45, 0, 0.55, 1],
  easeEmphasized: [0.16, 1, 0.3, 1],
  easeDrawer: [0.32, 0.72, 0, 1],
  pressScale: 0.96,
} as const;

export function visualStyleTokens(overrides: Partial<VisualStyleTokens>): VisualStyleTokens {
  return { ...SYNARA_TOKENS, ...overrides };
}
