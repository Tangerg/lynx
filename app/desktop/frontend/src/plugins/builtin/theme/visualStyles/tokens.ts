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
  | "wash-hover"
  | "wash-selected"
  | "app-drawer-surface"
  | "app-content-surface"
  | "app-header-surface"
  | "app-dock-surface"
  | "app-card-surface"
  | "app-surface-divider"
  | "app-card-edge"
  | "app-pane-split"
  | "app-pane-split-end"
  | "app-header-cast"
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
  // Corner ladder. Four values do all the work: 4 for anything that is really a
  // tag, 6 for controls and rows, 8 for cards, 10 for the surfaces that float.
  "style-shape-2xs": "3px",
  "style-shape-xs": "4px",
  "style-shape-sm": "6px",
  "style-shape-md": "8px",
  "style-shape-lg": "10px",
  "style-shape-xl": "12px",
  "style-shape-composer": "10px",
  "style-shape-bubble": "10px",
  "button-radius": "var(--shape-sm)",
  "field-radius": "var(--shape-md)",
  "segmented-radius": "var(--shape-md)",
  "segment-radius": "var(--shape-sm)",
  "surface-card-radius": "var(--shape-md)",
  "floating-panel-radius": "var(--shape-lg)",
  "floating-tip-radius": "var(--shape-sm)",
  "dock-tab-radius": "var(--shape-sm)",

  // Tool-window density: chrome rows are short and the type inside them carries
  // the hierarchy. Every height is even so a centred 1px rule never lands on a
  // half pixel.
  "control-height-xs": "22px",
  "control-height-sm": "26px",
  "control-height-md": "30px",
  "control-height-lg": "34px",
  "field-height-sm": "26px",
  "field-height-md": "28px",
  "field-height-lg": "32px",
  "menu-row-height": "30px",
  "dock-tab-height": "28px",
  "surface-header-height": "42px",
  "control-edge-width": "1px",
  "composer-edge-width": "1px",

  // Row states ride the SAME step as the surface ladder, so a hovered row is
  // exactly one rung of separation whatever the scheme and wherever the contrast
  // slider sits. Pinning them at fixed alphas is what made hover invisible on
  // dark and heavy-handed on light.
  "wash-hover": "color-mix(in srgb, var(--color-text) calc(var(--depth-step) * 0.75), transparent)",
  "wash-selected": "color-mix(in srgb, var(--color-text) var(--depth-step), transparent)",

  // ---- Regions -----------------------------------------------------------
  // Tool windows: three opaque materials and NO line anywhere between them. The
  // plane is the reading surface, the chrome columns step away from it, and each
  // seam is carried by a short directional cast from the column that overlaps.
  // A hairline here would draw the columns as a wireframe of pasted rectangles —
  // the value delta is the separation, the cast is the depth.
  "app-drawer-surface": "var(--color-surface)",
  "app-content-surface": "var(--color-bg)",
  // Bars inherit whatever region they sit in. A chrome bar is not a third
  // material stacked on the column; it is the top of the column.
  "app-header-surface": "transparent",
  // The dock recedes a quarter step back toward the plane, so the two flanks are
  // legible as near and far rather than as one frame in two pieces.
  "app-dock-surface": "color-mix(in oklab, var(--color-bg) 25%, var(--color-surface))",
  "app-card-surface": "var(--color-elevated)",
  "app-surface-divider": "transparent",
  // The drawer's cast, drawn INSIDE the plane: the plane outranks the drawer on
  // z-index so the drawer can slide under it, which means the drawer cannot cast
  // onto it from outside.
  //
  // GEOMETRY ONLY — a style says what shape an edge takes (a spread cast here, a
  // hairline `inset 1px 0 0 0` for a flat family), and the shell says how strong
  // it is right now by naming the colour on the element itself. Putting the
  // colour in the token looks equivalent and is not: the token is declared on
  // :root, so any var() in it resolves there, where the shell's live boundary
  // variables do not exist.
  "app-card-edge": "inset 11px 0 24px -10px",
  // A pane that splits the region it lives in casts across the split instead.
  "app-pane-split": "-7px 0 22px -10px",
  "app-pane-split-end": "7px 0 22px -10px",
  // The same cast, rotated: a bar that sits on the plane with the document
  // scrolling under it separates downward. Same triple as the pane split, so the
  // three seams around the reading plane are one mechanism at three angles rather
  // than two casts and a rule.
  "app-header-cast": "0 7px 22px -10px",
  // Reserved for the one place an optical ring still earns its pixel: a floating
  // panel, which has no value delta to lean on because it can land over anything.
  "seam-line": "color-mix(in oklab, var(--color-border) 82%, var(--color-text) 18%)",

  // ---- Elevation ---------------------------------------------------------
  // Flush chrome casts nothing. Only surfaces that genuinely leave the plane —
  // menus, popovers, tooltips — carry depth, and they carry it as one token.
  "shadow-border": "0 0 0 1px color-mix(in srgb, var(--color-text) 9%, transparent)",
  "shadow-control": "none",
  "shadow-composer-depth": "none",
  "shadow-popover": "0 6px 20px var(--shadow-cast)",
  "shadow-well": "none",
  "shadow-raised-chip": "none",
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
