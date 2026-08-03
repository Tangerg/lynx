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
  | "app-composer-surface"
  | "composer-backdrop"
  | "app-card-edge"
  | "app-pane-split"
  | "app-pane-split-end"
  | "app-header-edge"
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
  // Half a pixel: on a 2x panel that is exactly one device pixel, which is what
  // makes a hairline read as weightless rather than as an outline. At 1px the
  // composer was the heaviest edge on a screen whose regions separate by 3px of
  // cast.
  "composer-edge-width": "0.5px",

  // Row states ride the SAME step as the surface ladder, so a hovered row is
  // exactly one rung of separation whatever the scheme and wherever the contrast
  // slider sits. Pinning them at fixed alphas is what made hover invisible on
  // dark and heavy-handed on light.
  "wash-hover": "color-mix(in srgb, var(--color-text) calc(var(--depth-step) * 0.75), transparent)",
  "wash-selected": "color-mix(in srgb, var(--color-text) var(--depth-step), transparent)",

  // ---- Regions -----------------------------------------------------------
  // Tool windows: three opaque materials. The plane is the reading surface, the
  // chrome columns step away from it, and each seam is a single device pixel over
  // that step. The value delta is the separation; the hairline is what makes it
  // crisp instead of approximate. Both halves are load-bearing — the delta alone
  // measured too small to read, and a line alone would draw the columns as a
  // wireframe of pasted rectangles.
  "app-drawer-surface": "var(--color-surface)",
  "app-content-surface": "var(--color-bg)",
  // Bars inherit whatever region they sit in. A chrome bar is not a third
  // material stacked on the column; it is the top of the column.
  "app-header-surface": "transparent",
  // The dock recedes a quarter step back toward the plane, so the two flanks are
  // legible as near and far rather than as one frame in two pieces.
  "app-dock-surface": "color-mix(in oklab, var(--color-bg) 25%, var(--color-surface))",
  "app-card-surface": "var(--color-elevated)",
  // The composer's material, and the one translucent surface in the language. It
  // rests ON the transcript rather than in a column, so it picks up what passes
  // underneath — that, and not the ring, is what makes the ring read as the edge of
  // glass instead of as a stroke around a box. A flat style spells this opaque and
  // gets its bordered input back.
  "app-composer-surface": "color-mix(in oklab, var(--app-content-surface) 86%, transparent)",
  "composer-backdrop": "blur(20px) saturate(1.4)",
  // The drawer's boundary, drawn INSIDE the plane: the plane outranks the drawer on
  // z-index so the drawer can slide under it, which means the drawer cannot draw the
  // seam from outside.
  //
  // GEOMETRY ONLY — a style says what shape an edge takes and the shell says how
  // strong it is right now, by naming the colour on the element itself. Putting the
  // colour in the token looks equivalent and is not: the token is declared on :root,
  // so any var() in it resolves there, where the shell's live boundary variables do
  // not exist.
  //
  // HALF A PIXEL, NOT A CAST. These were directional casts, and a cast spreads onto
  // the reading plane — which is a shadow falling on paper, so all three seams read
  // as pressing down on the document. The reference draws every one of them as a
  // single device pixel instead (measured at 2x: 207 down the vertical seam, 225
  // under a chrome bar, against a 255 plane) and keeps the material delta underneath.
  // One primitive for the whole screen, the same one the composer's edge uses: an
  // edge that is crisp and carries no weight. The two weights are OUR ramp, and the
  // distinction is real — a column change earns `border-soft`, a bar inside one
  // region earns `border`.
  "app-card-edge": "inset 0.5px 0 0 0",
  // A pane that splits the region it lives in carries the line on its own leading
  // edge; `-end` is the mirror, for a pane docked to the other side.
  "app-pane-split": "inset 0.5px 0 0 0",
  "app-pane-split-end": "inset -0.5px 0 0 0",
  // The bottom edge of a chrome bar.
  "app-header-edge": "inset 0 -0.5px 0 0",
  // Reserved for the one place an optical ring still earns its pixel: a floating
  // panel, which has no value delta to lean on because it can land over anything.
  "seam-line": "color-mix(in oklab, var(--color-border) 82%, var(--color-text) 18%)",

  // ---- Elevation ---------------------------------------------------------
  // Flush chrome casts nothing. Only surfaces that genuinely leave the plane —
  // menus, popovers, tooltips — carry depth, and they carry it as one token.
  "shadow-border": "0 0 0 1px color-mix(in srgb, var(--color-text) 9%, transparent)",
  "shadow-control": "none",
  // Flush chrome casts nothing — but the composer is not flush. Two layers, and the
  // first has no offset on purpose: an ambient bloom on all four sides is what makes
  // a half-pixel ring legible at all (with depth alone below it, the top and side
  // edges measured 1-2 levels off the plane). Calibrated against the reference: its
  // halo reaches 246 against a 255 plane and fades over ~22px, where a first attempt
  // at 230 over 11px read as a drop shadow. The second layer is the drop.
  // Depth only; the ring beside it is drawn from `--composer-edge-width` at the
  // callsite, where the colour can answer focus.
  "shadow-composer-depth": "0 0 26px -8px var(--shadow-cast), 0 6px 22px -12px var(--shadow-cast)",
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
