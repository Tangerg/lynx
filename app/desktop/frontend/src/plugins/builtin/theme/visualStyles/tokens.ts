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
  | "row-radius"
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
  | "app-dock-tabstrip-surface"
  | "dock-tab-active-surface"
  | "app-card-surface"
  | "app-composer-surface"
  | "app-composer-tray-surface"
  | "app-composer-project-tray-surface"
  | "app-floating-surface"
  | "composer-backdrop"
  | "composer-tray-backdrop"
  | "composer-tray-edge-color"
  | "floating-backdrop"
  | "app-card-edge"
  | "app-pane-split"
  | "app-pane-split-end"
  | "app-header-edge"
  | "app-pane-wash"
  | "app-pane-wash-end"
  | "seam-line"
  | "shadow-control"
  | "shadow-composer-depth"
  | "shadow-ring"
  | "shadow-raised"
  | "shadow-overlay"
  | "shadow-popover"
  | "shadow-modal"
  | "shadow-well"
  | "shadow-raised-chip"
  | "shadow-surface-card";

export type VisualStyleTokens = Record<VisualStyleTokenName, string>;

export const WORKBENCH_TOKENS: VisualStyleTokens = {
  // Corner ladder. Four values do all the work: 4 for anything that is really a
  // tag, 6 for controls and rows, 8 for cards, 10 for the surfaces that float.
  "style-shape-2xs": "2px",
  "style-shape-xs": "4px",
  "style-shape-sm": "6px",
  "style-shape-md": "8px",
  "style-shape-lg": "10px",
  "style-shape-xl": "12px",
  // The composer is the roundest object in the window, and deliberately rounder
  // than anything that floats: the reference runs 22px at rest and 20px once the
  // text wraps, against 12–16 for its panels. Ours sat on the floating rung, which
  // made the one surface you type into look like another panel. One value for both
  // states — the 2px the reference moves between them needs the wrapped line count,
  // and that is a measurement, not a style.
  "style-shape-composer": "20px",
  // A bubble is the one object that wears the widest corner. It is small, floated
  // to one side and quoted rather than read straight through, and at the card radius
  // a narrow bubble read as a cropped card. The reference uses its 2xl (16px) here.
  "style-shape-bubble": "16px",
  "button-radius": "var(--shape-sm)",
  "field-radius": "var(--shape-md)",
  "segmented-radius": "var(--shape-md)",
  "segment-radius": "var(--shape-sm)",
  "surface-card-radius": "var(--shape-md)",
  // A row is not a small card. It is a full-bleed target whose corner exists only
  // so a wash sliding under the cursor has a shape, and at the card's rung that
  // wash read as a stack of stubby cards in a list. Every desktop reference gives
  // the row its own rung and puts it ABOVE the card's — 10px on a 30px row.
  "row-radius": "var(--shape-lg)",
  "floating-panel-radius": "var(--shape-xl)",
  "floating-tip-radius": "var(--shape-sm)",
  // A tab is the top of the panel under it, so it takes the surface rung rather
  // than a control's.
  "dock-tab-radius": "var(--shape-lg)",

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
  // slider sits. Fixed alphas cannot preserve that rung across light and dark schemes.
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
  // The dock's tab strip is the one bar that is NOT the ground of what it labels.
  // A tab is the top edge of its panel, so the panel's ground has to reach up into
  // the selected tab — which only reads if the strip itself sits a step back. The
  // step is the contrast preference, not a picked value.
  "app-dock-tabstrip-surface":
    "color-mix(in oklab, var(--color-text) calc(var(--depth-step) * 0.75), var(--app-dock-surface))",
  // What the selected tab fills with. Its identity with the panel's ground IS the
  // tab metaphor; a style that would rather have a chip or an underline retargets
  // this one value (see `data-control-treatment`).
  "dock-tab-active-surface": "var(--app-dock-surface)",
  "app-card-surface": "var(--color-elevated)",
  // The composer's material, and the one translucent surface in the language. It
  // rests ON the transcript rather than in a column, so it picks up what passes
  // underneath — that, and not the ring, is what makes the ring read as the edge of
  // glass instead of as a stroke around a box. A flat style spells this opaque and
  // gets its bordered input back.
  "app-composer-surface": "color-mix(in oklab, var(--app-content-surface) 86%, transparent)",
  "composer-backdrop": "blur(20px) saturate(1.4)",
  "app-composer-tray-surface": "color-mix(in oklab, var(--app-content-surface) 70%, transparent)",
  // The project picker is the home utility surface BEHIND the glass composer,
  // not standing Goal/Plan material above it. Codex gives that rear plane one
  // opaque 4% ink step so the foreground composer can overlap it cleanly.
  "app-composer-project-tray-surface":
    "color-mix(in srgb, var(--color-text) 4%, var(--app-content-surface))",
  "composer-tray-backdrop": "blur(8px)",
  "composer-tray-edge-color": "color-mix(in oklab, var(--color-border) 80%, transparent)",
  // A floating panel is the other translucent surface. Same reason as the composer:
  // it lets a hint of what it covers through, so it reads as glass over the page
  // rather than as a second page pasted on it. Held here rather than as an alpha at
  // the atom, because how see-through a panel is is the style's call, not the
  // panel's.
  // 90%, not 70%. A floating panel lets a HINT of the page through; at 30%
  // transparent it was letting the page through, and the reference — whose whole
  // popover language this one already follows (half-pixel ring, one drop, a blur
  // behind) — sits at 10%. The pair matters together: ours was three times more
  // see-through AND five times more blurred, which is the look this design
  // language spends a rule forbidding everywhere else.
  "app-floating-surface": "color-mix(in oklab, var(--app-content-surface) 90%, transparent)",
  // The other half of the glass, and it belongs here rather than in the atom for
  // the same reason the opacity does: how see-through a panel is is the style's
  // call, and half a recipe in a token with the other half in a utility class is
  // how the two drift.
  "floating-backdrop": "blur(8px) saturate(1.4)",
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
  // The seam has a second half, and it lives on the CHROME. The reference darkens its
  // sidebar inward as it approaches the split — 248 to 238 over 12px, measured — and
  // only then draws the hairline. That is why its seam reads soft without reading as
  // pressure: the gradient falls on the panel, not on the page. The earlier revision
  // did the opposite and lit the reading plane.
  // `-end` is the mirror, for chrome that sits BEFORE the plane (the drawer, the
  // settings nav) rather than after it (the dock, the review navigator).
  "app-pane-wash": "inset 12px 0 12px -6px",
  "app-pane-wash-end": "inset -12px 0 12px -6px",
  // Reserved for the one place an optical ring still earns its pixel: a floating
  // panel, which has no value delta to lean on because it can land over anything.
  // A floating surface's edge, and it is the region step itself — the reference
  // draws its `elevation-stroke` at exactly the same weight as its heaviest border
  // (12% of the ink) and adds nothing on top.
  "seam-line": "var(--color-border-soft)",

  // ---- Elevation ---------------------------------------------------------
  // Flush chrome casts nothing. Only surfaces that genuinely leave the plane —
  // menus, popovers, tooltips — carry depth, and they carry it as one token.
  "shadow-control": "none",
  // Flush chrome casts nothing — but the composer is not flush. Two layers, and the
  // first has no offset on purpose: an ambient bloom on all four sides is what makes
  // a half-pixel ring legible at all (with depth alone below it, the top and side
  // edges measured 1-2 levels off the plane). Calibrated against the reference: its
  // halo reaches 246 against a 255 plane and fades over ~22px, where a first attempt
  // at 230 over 11px read as a drop shadow. The second layer is the drop.
  // Depth only; the ring beside it is drawn from `--composer-edge-width` at the
  // callsite, where the colour can answer focus.
  "shadow-composer-depth":
    "0 8px 16px -4px color-mix(in oklab, var(--shadow-cast) 60%, transparent), 0 0 26px -8px var(--shadow-cast)",
  // Four heights, each a ring plus a drop; see globals.css for the rule and for why
  // they are not called `shadow-sm/lg/xl` (those are Tailwind's own theme keys).
  "shadow-ring": "0 0 0 0.5px var(--seam-line)",
  "shadow-raised":
    "var(--shadow-ring), 0 1px 2px -1px color-mix(in oklab, var(--shadow-cast) 40%, transparent)",
  "shadow-overlay":
    "var(--shadow-ring), 0 4px 8px -2px color-mix(in oklab, var(--shadow-cast) 50%, transparent)",
  "shadow-popover":
    "var(--shadow-ring), 0 8px 16px -4px color-mix(in oklab, var(--shadow-cast) 60%, transparent)",
  "shadow-modal":
    "var(--shadow-ring), 0 16px 32px -8px color-mix(in oklab, var(--shadow-cast) 95%, transparent)",
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
  // The App Shell changes two things at once: a flank travels and the reading plane
  // takes its measure. Codex drives that relationship from one interruptible progress
  // value with a 500ms / 0.1-bounce spring. These samples are that spring at 25ms
  // intervals, published as native CSS `linear()` so the browser owns interruption and
  // reversal without subscribing React to animation frames.
  //
  // A trace of the previous 300ms near-uniform curve on the long transcript dropped no
  // frames (max rAF interval 16.7ms), but at 150ms it was only halfway across: the text
  // kept reflowing at visible speed for the whole gesture and read as drag. The spring
  // reaches 54% / 90% / 99% at 100 / 200 / 300ms, then spends its remaining clock on a
  // sub-pixel settle. It is both more immediate and easier to track even though its
  // formal duration is longer.
  drawerMs: 500,
  easeOut: [0.22, 1, 0.36, 1],
  easeInOut: [0.45, 0, 0.55, 1],
  easeEmphasized: [0.16, 1, 0.3, 1],
  // `--ease-drawer` in globals.css mirrors these values for the frame before the visual
  // style is published. A sampled spring, rather than a fitted cubic, preserves the
  // reference's tiny overshoot; native CSS transition reversal keeps an interrupted
  // gesture continuous without adding an animation-frame owner in React.
  drawerProgress: [
    0, 0.06981, 0.21761, 0.38345, 0.53716, 0.66615, 0.76765, 0.84375, 0.89859, 0.93672, 0.96233,
    0.97894, 0.98929, 0.99544, 0.99887, 1.00061, 1.00135, 1.00152, 1.00142, 1.00119, 1,
  ],
  pressScale: 0.98,
} as const;

export function visualStyleTokens(overrides: Partial<VisualStyleTokens>): VisualStyleTokens {
  return { ...WORKBENCH_TOKENS, ...overrides };
}
