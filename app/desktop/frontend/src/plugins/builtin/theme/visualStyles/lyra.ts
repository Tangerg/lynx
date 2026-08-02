import type { VisualStyleSpec } from "@/plugins/sdk";
import { SYNARA_MOTION, visualStyleTokens } from "./tokens";

/** The two chrome columns: one tone step off the reading plane, taken entirely as
 *  a trace of the user's accent.
 *
 *  A single mix rather than an ink step plus a tint. The accent already carries
 *  the step — it is darker than the canvas on light and lighter on dark, so
 *  "toward the accent" resolves to "away from the reading plane" in both schemes
 *  on its own, and stacking a neutral step on top of it only overshoots the
 *  contrast the quiet ink rungs are calibrated against. Taking the step from a
 *  hue rather than from grey is also what keeps it from reading as dirt: a
 *  neutral wash this faint rounds off to a dead grey, and dead grey beside white
 *  looks like a smudge instead of a surface. */
const CHROME_COLUMN = "color-mix(in oklab, var(--color-bg) 96%, var(--color-accent) 4%)";

/** What separates the reading plane from a chrome column: a soft cast, not a line.
 *  Wide and heavily negative-spread so the boundary is a ~20px gradient the eye
 *  reads as depth, instead of a 1px cut it reads as a seam between two collaged
 *  rectangles. Black-based rather than ink-based because a cast is an absence of
 *  light in both schemes. */
const COLUMN_CAST = "30px -28px color-mix(in srgb, black 18%, transparent)";

export const lyraStyle: VisualStyleSpec = {
  id: "lyra",
  label: "Lyra",
  description:
    "Tonal columns around a raised reading plane. Depth by tone and cast, never by lines.",
  order: -10,
  traits: { regions: "tonal-columns", controls: "outlined" },
  motion: SYNARA_MOTION,
  preview: {
    canvas: "#ffffff",
    sidebar: "#eff0f3",
    dock: "#eff0f3",
    edge: "rgb(0 0 0 / 0.1)",
    accent: "#006bff",
  },
  tokens: visualStyleTokens({
    "surface-header-height": "52px",
    "dock-tab-height": "30px",
    // Three columns: the conversation is the lit plane, both chrome columns share
    // one recessed tone. Symmetric on purpose — the drawer and the dock are the
    // same kind of region, and answering them with a ring on one side and a
    // hairline on the other is what stopped the shell reading as three columns.
    "app-drawer-surface": CHROME_COLUMN,
    "app-dock-surface": CHROME_COLUMN,
    "app-card-surface": CHROME_COLUMN,
    "app-content-surface": "var(--color-bg)",
    "app-header-surface": "transparent",

    // No frost: the columns are opaque material, so a blur behind them would be
    // sampling a surface nothing shows through.
    "app-drawer-backdrop": "none",
    "app-drawer-shadow": "none",
    "app-drawer-sheen-opacity": "0",

    // Every boundary in the shell is a cast from the reading plane outward.
    "app-content-shadow": `-16px 0 ${COLUMN_CAST}`,
    "app-pane-split": `inset 16px 0 ${COLUMN_CAST}`,
    "app-pane-split-end": `inset -16px 0 ${COLUMN_CAST}`,

    // …and nothing in the shell is a line.
    "app-content-ring": "none",
    "app-content-ring-opacity": "0",
    "app-surface-divider": "transparent",
    "seam-line": "transparent",
    "seam-shadow-color": "color-mix(in srgb, black 18%, transparent)",

    // A plane that spans its column edge to edge has no corner to round.
    "style-shape-content": "0px",
  }),
};
