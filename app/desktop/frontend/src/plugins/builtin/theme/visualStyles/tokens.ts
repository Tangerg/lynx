type VisualStyleTokenName =
  | "style-shape-2xs"
  | "style-shape-xs"
  | "style-shape-sm"
  | "style-shape-md"
  | "style-shape-lg"
  | "style-shape-xl"
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
  | "app-content-surface"
  | "app-header-surface"
  | "app-dock-surface"
  | "app-card-surface"
  | "app-surface-divider"
  | "app-pane-split"
  | "app-pane-split-end"
  | "seam-line"
  | "shadow-border"
  | "shadow-control"
  | "shadow-composer-depth"
  | "shadow-popover"
  | "shadow-well"
  | "shadow-raised-chip"
  | "shadow-surface-card";

export type VisualStyleTokens = Record<VisualStyleTokenName, string>;

export const WORKBENCH_TOKENS: VisualStyleTokens = {
  "style-shape-2xs": "3px",
  "style-shape-xs": "4px",
  "style-shape-sm": "6px",
  "style-shape-md": "8px",
  "style-shape-lg": "10px",
  "style-shape-xl": "12px",
  "style-shape-composer": "10px",
  "style-shape-bubble": "8px",
  "button-radius": "var(--shape-sm)",
  "field-radius": "var(--shape-sm)",
  "segmented-radius": "var(--shape-sm)",
  "segment-radius": "var(--shape-xs)",
  "surface-card-radius": "var(--shape-md)",
  "floating-panel-radius": "var(--shape-lg)",
  "floating-tip-radius": "var(--shape-sm)",
  "dock-tab-radius": "var(--shape-xs)",
  "control-height-xs": "24px",
  "control-height-sm": "28px",
  "control-height-md": "32px",
  "control-height-lg": "38px",
  "field-height-sm": "26px",
  "field-height-md": "32px",
  "field-height-lg": "36px",
  "menu-row-height": "28px",
  "dock-tab-height": "28px",
  "surface-header-height": "46px",
  "control-edge-width": "1px",
  "composer-edge-width": "1px",
  "app-drawer-surface": "var(--color-surface-2)",
  "app-content-surface": "var(--color-bg)",
  "app-header-surface": "var(--color-surface-2)",
  "app-dock-surface": "var(--color-surface-2)",
  "app-card-surface": "var(--color-surface-2)",
  "app-surface-divider": "color-mix(in oklab, var(--color-border) 82%, var(--color-text) 18%)",
  "app-pane-split": "inset 1px 0 0 0 var(--app-surface-divider)",
  "app-pane-split-end": "inset -1px 0 0 0 var(--app-surface-divider)",
  "seam-line": "var(--app-surface-divider)",
  "shadow-border": "0 0 0 1px color-mix(in srgb, var(--color-text) 9%, transparent)",
  "shadow-control":
    "0 0 0 0.5px color-mix(in srgb, var(--color-text) 9%, transparent), 0 1px 1px color-mix(in srgb, var(--color-text) 4%, transparent), 0 4px 10px -4px color-mix(in srgb, var(--color-text) 7%, transparent)",
  "shadow-composer-depth":
    "0 8px 24px -18px color-mix(in srgb, var(--color-text) 22%, transparent)",
  "shadow-popover":
    "0 0 0 1px var(--seam-line), 0 10px 30px -10px color-mix(in srgb, var(--color-text) 14%, transparent)",
  "shadow-well": "inset 0 1px 2px color-mix(in srgb, var(--color-text) 6%, transparent)",
  "shadow-raised-chip":
    "0 1px 1.5px color-mix(in srgb, var(--color-text) 6%, transparent), inset 0 1px 0 color-mix(in srgb, white 45%, transparent)",
  "shadow-surface-card": "none",
};

export const WORKBENCH_MOTION = {
  instantMs: 80,
  fastMs: 150,
  mediumMs: 200,
  disclosureMs: 220,
  slowMs: 360,
  drawerMs: 240,
  easeOut: [0.22, 1, 0.36, 1],
  easeInOut: [0.45, 0, 0.55, 1],
  easeEmphasized: [0.16, 1, 0.3, 1],
  easeDrawer: [0.32, 0.72, 0, 1],
  pressScale: 0.98,
} as const;

export function visualStyleTokens(overrides: Partial<VisualStyleTokens>): VisualStyleTokens {
  return { ...WORKBENCH_TOKENS, ...overrides };
}
