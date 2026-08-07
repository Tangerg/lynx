---
version: 1.0
name: lyra-design-analysis
description: "Lyra is an agent client — a desktop chat shell that streams Lyra Runtime Protocol events from a Go runtime. CURRENT DIRECTION (2026-07, drawer + content card): the work index is a fixed-position DRAWER plus an in-flow spacer, and the content is a card that floats over it — card z-index outranks the drawer, so collapsing slides the drawer UNDER the card and the card never moves. The two regions sit at nearly the same value and the split is carried by ONE 1px inset ring on the card (clipped to the seam-side radius) plus a low-spread directional shadow; internal pane splits and chrome-bar bottoms use the more-transparent --app-surface-divider hairline. This SUPERSEDES the earlier background-delta / no-hairline model. One chrome-bar height (46px) is shared by the drawer and every content header so they align across the seam. Type and corner radius are derived ladders (--fs-* from one base size, --shape-* off a 10px base × the user Shape scale) — no per-callsite pixel values; check:tokens fails the build on any. Light by default with full dark parity, bundled Geist Sans/Mono, one restrained blue-700 accent reserved for live/focus/links, inverting ink primary CTA. AUTHORITATIVE VALUES LIVE IN CODE: src/styles/globals.css and src/plugins/builtin/theme/themes/*.ts — the YAML below is historical illustration from the dark-first spec; trust the code."

colors:
  # ---- Accent ----
  # One restrained, near-monochrome chromatic accent — used scarcely. Allowed
  # surfaces: active tab indicator, primary CTA, focus ring (a single thin
  # stroke, no bright halo), live indicator (streaming dot, running pill).
  # Forbidden as section background, card fill, or decorative tint. The hue is
  # user-selectable; the default is a calm blue (green is now just one option).
  accent: "#6c97ff"             # default accent — calm blue (dark); #2563eb on light
  accent-border: "#5b86f0"
  accent-pressed: "#4a72d8"
  on-accent: "#ffffff"

  # ---- Ink (text) ----
  ink: "#f7f8f8"                # Headlines + emphasized body
  ink-soft: "#d0d6e0"            # Body / paragraph
  ink-muted: "#8a8f98"           # Secondary / inactive nav / meta
  ink-faint: "#62666d"           # Tertiary / disabled / footnotes

  # ---- Surface ladder ----
  # Flush layout: canvas IS the main reading area; surface is the one lifted
  # chrome step (sidebar, cards, bubbles), divided from canvas by a hairline.
  # -2/-3/-4 derive via color-mix so the contrast slider moves them per scheme.
  canvas: "#0c0d0f"              # Main reading surface (dark). Light: #ffffff
  surface: "#16181b"             # Lifted chrome — sidebar / cards / bubble. Light: #f6f7f8
  surface-2: "#1c1e21"           # Hover / active row, raised surface (derived)
  surface-3: "#212327"           # Sub-nav, dropdown, popover (derived)
  surface-4: "#26282c"           # Deepest lifted surface (derived)

  # ---- Hairlines ----
  hairline: "#23252a"            # Default 1px border
  hairline-strong: "#34343a"     # Input focus, emphasized divider
  hairline-tertiary: "#3e3e44"   # Nested surface borders

  # ---- Semantic ----
  # Used ONLY for genuine errors / warnings / live confirmations. Not
  # decoration. RUN_ERROR banner / approval-card warnings / status dots.
  # Dark-tuned: desaturated + lifted vs the raw web values so they don't
  # vibrate / edge-bleed on the near-black canvas (Apple Dark Mode / Ant
  # dark). Light themes keep the saturated web values (read clean on white).
  success: "#3fb950"             # Confirmed action, run finished cleanly
  warning: "#f0a936"             # User attention required (approval pending)
  negative: "#f85149"            # Errors (RUN_ERROR banner, tool failure)
  info: "#58a6ff"                # Inline links, info badges

  # ---- Light theme (full parity, not second-class) ----
  light-canvas: "#ffffff"        # clean white main reading area
  light-surface: "#f6f7f8"       # subtle gray chrome — sidebar / cards
  light-hairline: "#ebebeb"
  light-hairline-strong: "#d4d4d6"
  light-ink: "#171717"
  light-ink-soft: "#4d4d4d"
  light-ink-muted: "#5e5e5e"
  light-accent: "#2563eb"        # accent reads crisp on white

typography:
  # ---- Font families ----
  # Sans: the native OS UI face (SF Pro on macOS via -apple-system, PingFang
  # for CJK) — the crisp, premium, native default; no bundled webfont.
  # Mono: the native OS monospace (SF Mono / Menlo) — code / IDs / timestamps /
  # paths only (NOT every eyebrow). Single --font-sans / --font-mono token in
  # globals.css; the user can override either in Settings → Appearance.

  # ---- Display ----
  # 600 is the display ceiling. Both Linear and Vercel forbid 700+.
  # Negative tracking on display, near-zero on body.
  display-xl:
    fontFamily: -apple-system, BlinkMacSystemFont, SF Pro Text, system-ui, PingFang SC, sans-serif
    fontSize: 32px
    fontWeight: 600
    lineHeight: 1.10
    letterSpacing: -0.96px
  display-lg:
    fontFamily: -apple-system, BlinkMacSystemFont, SF Pro Text, system-ui, PingFang SC, sans-serif
    fontSize: 24px
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: -0.6px
  display-md:
    fontFamily: -apple-system, BlinkMacSystemFont, SF Pro Text, system-ui, PingFang SC, sans-serif
    fontSize: 20px
    fontWeight: 600
    lineHeight: 1.20
    letterSpacing: -0.4px
  display-sm:
    fontFamily: -apple-system, BlinkMacSystemFont, SF Pro Text, system-ui, PingFang SC, sans-serif
    fontSize: 16px
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: -0.2px

  # ---- Body ----
  body-lg:
    fontFamily: -apple-system, BlinkMacSystemFont, SF Pro Text, system-ui, PingFang SC, sans-serif
    fontSize: 15px
    fontWeight: 400
    lineHeight: 1.65
    letterSpacing: -0.1px
  body-md:
    fontFamily: -apple-system, BlinkMacSystemFont, SF Pro Text, system-ui, PingFang SC, sans-serif
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.55
    letterSpacing: -0.05px
  body-sm:
    fontFamily: -apple-system, BlinkMacSystemFont, SF Pro Text, system-ui, PingFang SC, sans-serif
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.50
    letterSpacing: 0
  body-xs:
    fontFamily: -apple-system, BlinkMacSystemFont, SF Pro Text, system-ui, PingFang SC, sans-serif
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.45
    letterSpacing: 0

  # ---- Button label ----
  button-md:
    fontFamily: -apple-system, BlinkMacSystemFont, SF Pro Text, system-ui, PingFang SC, sans-serif
    fontSize: 13px
    fontWeight: 500
    lineHeight: 1.20
    letterSpacing: 0
  button-sm:
    fontFamily: -apple-system, BlinkMacSystemFont, SF Pro Text, system-ui, PingFang SC, sans-serif
    fontSize: 12px
    fontWeight: 500
    lineHeight: 1.20
    letterSpacing: 0

  # ---- Caption / mono eyebrow ----
  # Replaces every ALL-CAPS + letter-spacing label from the previous system.
  # Mono signals "technical / observable / data" — used for reasoning headers,
  # tool-call signatures, file paths, durations, IDs (data only — not labels).
  caption:
    fontFamily: -apple-system, BlinkMacSystemFont, SF Pro Text, system-ui, PingFang SC, sans-serif
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.40
    letterSpacing: 0
  caption-mono:
    fontFamily: ui-monospace, SF Mono, SFMono-Regular, Menlo, monospace
    fontSize: 11.5px
    fontWeight: 400
    lineHeight: 1.40
    letterSpacing: 0
    fontFeatureSettings: "\"tnum\""
  code:
    fontFamily: ui-monospace, SF Mono, SFMono-Regular, Menlo, monospace
    fontSize: 12.5px
    fontWeight: 400
    lineHeight: 1.55
    letterSpacing: 0
    fontFeatureSettings: "\"tnum\""

rounded:
  none: 0px
  xs: 4px        # Badges, status pills
  sm: 6px        # Inputs, small buttons, nav buttons (Vercel --geist-radius)
  md: 8px        # Default button, card chrome, dialog (Linear --geist-marketing-radius)
  lg: 12px       # Workspace cards, pricing-style summaries
  xl: 16px       # Hero / lightbox frame
  pill: 9999px   # Status badge / segmented toggle ONLY — NEVER for CTAs
  circle: 50%    # Avatar, dot indicator

spacing:
  # 4-base — every value a multiple of 4. Both Linear & Vercel agree.
  px: 1px
  xxs: 4px
  xs: 8px
  sm: 12px
  md: 16px
  lg: 24px
  xl: 32px
  2xl: 40px
  3xl: 48px
  4xl: 64px
  5xl: 96px

# ---- Lyra-specific layout constants ----
layout:
  content-max: 720px       # Max reading width for chat content (was 760; narrowed 2026-06)
  sidebar-expanded: 248px  # Expanded sidebar (default state)
  sidebar-rail: 56px       # Collapsed icon rail (on demand, ⌘B)
  # No tab strip, no sidebar/main divider (separation is a background delta),
  # no bottom status bar — run telemetry lives in the composer footer, global
  # status/notifications in the sidebar footer.

motion:
  ease-out: cubic-bezier(0.3, 0, 0, 1)
  ease-emphasized: cubic-bezier(0.16, 1, 0.3, 1)
  ease-in-out: cubic-bezier(0.45, 0, 0.55, 1)
  dur-instant: 80ms
  dur-fast: 140ms
  dur-med: 220ms
  dur-slow: 360ms

components:
  # ---- Buttons ----
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    typography: "{typography.button-md}"
    rounded: "{rounded.md}"
    padding: "8px 14px"
    description: Primary CTA. Lyra signature green. Reserved for explicit action ("Send", "Approve", "Run").
  button-secondary:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink}"
    borderColor: "{colors.hairline}"
    typography: "{typography.button-md}"
    rounded: "{rounded.md}"
    padding: "8px 14px"
    description: Charcoal button on hairline border. Most "Cancel" / "Dismiss" / inline actions.
  button-tertiary:
    backgroundColor: transparent
    textColor: "{colors.ink-muted}"
    typography: "{typography.button-sm}"
    rounded: "{rounded.sm}"
    padding: "6px 8px"
    description: Plain text button. Sidebar toggles, inline minor actions.
  icon-button:
    backgroundColor: transparent
    textColor: "{colors.ink-muted}"
    rounded: "{rounded.sm}"
    minSize: 28px
    description: Square icon container. 28×28 standard, 32×32 emphasized. Hover surface-2 fill.

  # ---- Composer surface ----
  composer:
    backgroundColor: "{colors.surface-1}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.lg}"
    padding: "12px 14px"
    maxWidth: "{layout.chat-measure}"
    description: Textarea + toolbar surface. Anchored bottom, centered to chat-measure. One real border defines the edge; a depth-only shadow lifts it; focus quietly strengthens the border.
  composer-chip:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.ink-muted}"
    typography: "{typography.caption-mono}"
    rounded: "{rounded.xs}"
    padding: "2px 8px"
    description: Attachment / file ref pill. Mono caption — these are file paths and IDs.
  segmented-control:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.ink-muted}"
    rounded: "{rounded.sm}"
    description: Composer mode picker (Agent / Ask / Plan). Active segment lifts to surface-3 + ink.

  # ---- Message stream ----
  message-bubble-user:
    backgroundColor: "{colors.surface-1}"
    borderColor: "{colors.hairline}"
    textColor: "{colors.ink}"
    typography: "{typography.body-md}"
    rounded: "14px 14px 4px 14px"
    padding: "10px 14px"
    maxWidth: "580px"
    description: Right-aligned bubble. Compact, hairline-bordered.
  message-body-assistant:
    backgroundColor: transparent
    textColor: "{colors.ink-soft}"
    typography: "{typography.body-md}"
    maxWidth: "{layout.chat-measure}"
    description: Full-width prose. No bubble chrome. Avatar peeks left.
  reasoning-block:
    backgroundColor: "{colors.surface-2}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
    headerTypography: "{typography.caption-mono}"
    headerColor: "{colors.ink-muted}"
    bodyTypography: "{typography.body-sm}"
    bodyColor: "{colors.ink-muted}"
    bodyFontStyle: italic
    description: Collapsible thinking panel — a filled surface-2 box (chosen over a left-border: it reads as a distinct, hover-able disclosure with an auto-expand-while-streaming header). Header shows "thinking · 12s" or "thought for 12s" in mono lowercase (NEVER "THINKING" all-caps).
  tool-call-card:
    backgroundColor: "{colors.surface-1}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.md}"
    signatureTypography: "{typography.code}"
    metaTypography: "{typography.caption-mono}"
    description: |
      Renders like an RPC log entry, not a generic card. First line: function
      signature in mono (e.g. `read_file(path: "src/auth.ts")`). Second line:
      status glyph + duration + bytes/lines summary in caption-mono. Expandable
      for full result.

  # ---- Code & Mermaid ----
  shiki-code-block:
    backgroundColor: "{colors.surface-1}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.md}"
    headTypography: "{typography.caption-mono}"
    bodyTypography: "{typography.code}"
    description: Shiki-highlighted code with mono header (lang lowercase, optional filename, copy button on hover). Long blocks auto-collapse > 24 lines.
  mermaid-block:
    backgroundColor: "{colors.surface-1}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.md}"
    description: Clickable diagram — click opens lightbox at native scale. Diagram colors derived from theme tokens at render time.

  # ---- Navigation ----
  # (2026-06) chat-tab + view-tab removed — no tab strip. One active session;
  # workspace views open full-pane.
  command-palette:
    backgroundColor: "{colors.surface-2}"
    borderColor: "{colors.hairline-strong}"
    rounded: "{rounded.lg}"
    backdropFilter: blur(10px)
    itemTypography: "{typography.body-sm}"
    description: ⌘K overlay. Surface-2 + hairline-strong + backdrop blur + Level 5 stacked shadow (the rare floating element where shadow is allowed).

  # ---- Overlays ----
  toast:
    backgroundColor: "{colors.surface-2}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.md}"
    typography: "{typography.body-sm}"
    description: Plugin toaster entry. Bottom-right stack. Auto-dismiss 4s.
  approval-card:
    backgroundColor: "color-mix(in srgb, {colors.warning} 8%, {colors.surface-1})"
    borderColor: "color-mix(in srgb, {colors.warning} 30%, transparent)"
    rounded: "{rounded.md}"
    titleTypography: "{typography.display-sm}"
    metaTypography: "{typography.caption-mono}"
    description: HITL approval prompt. Warning-tinted card with mono command preview, "Approve" primary + "Decline" secondary.
  run-error-banner:
    backgroundColor: "color-mix(in srgb, {colors.negative} 12%, transparent)"
    borderColor: "color-mix(in srgb, {colors.negative} 35%, transparent)"
    rounded: "{rounded.md}"
    titleTypography: "{typography.caption-mono}"
    titleColor: "{colors.negative}"
    bodyTypography: "{typography.body-sm}"
    description: Run-error surface. Lives above the message stream, dismissible. Cleared automatically when the next run starts.
---

> **Shell rewrite 2026-07 (landed on `main`) — read this first.** The structural
> model below is superseded on these points:
> - **Drawer, not a grid column.** The work index is `position: fixed` and slides;
>   an in-flow spacer reserves its width. A grid template swap cannot be
>   interpolated, which is why the old shell snapped on collapse.
> - **Content is a card over the drawer.** Rounded on the seam side, one 1px inset
>   ring clipped to that side's radius, one low-spread directional shadow, and the
>   parent backs the rounded-corner wedge with the drawer's own material. Collapsed,
>   the card squares off and drops ring + shadow so its corner cannot double up
>   with the OS window's.
> - **Separation is a hairline, NOT a background delta.** Drawer and card sit half
>   a step apart on purpose; the ring does the dividing. The earlier "no grey
>   rules, background delta only" rule is reversed — see DESKTOP_UI_POLISH.md §2.
> - **One chrome-bar height** (`--surface-header-height`, 46px) shared by the
>   drawer header and every content header, so they line up across the seam.
> - **Composer floats over the transcript** (`-mt-5`), in normal flow. No reserved
>   bottom padding, no gradient mask.
> - **Composer owns one real edge.** Its 1px field border defines the shape,
>   focus only strengthens that border, and `--shadow-composer-depth` carries
>   depth without drawing a second optical ring.
> - **Derived ladders.** Type (`--fs-*`) comes from one base size; radius
>   (`--shape-*`) from a 10px base × the user's Shape scale. Density (`--density-*`)
>   is a third, independent axis. Per-callsite pixel values are a build failure.
>
> **Redesign 2026-06 (landed on `main`).** The OpenAI-restrained redesign
> changed the structural design intent from the original spec below:
> - **Tabs removed** — one active session; workspace views open full-pane and
>   close via sidebar-toggle / `Esc` (no tab strip, no in-view ×).
> - **Sidebar/main divider removed** — separation is a background delta only
>   (`surface` vs `bg`), no hairline.
> - **Display weight ceiling lowered 600 → 500** (Codex-style restraint).
> - **Assistant message de-chromed** — no glass document surface, no per-message
>   header/avatar, no `MessageOutline` gutter (unboxed prose on the canvas).
> - **Composer is the `rounded-xl` anchor** with `bg-fg` send (accent reserved
>   for live/steer state); model picker + context chips moved inside.
> - Shadow roles are semantic: composer depth, popover edge + depth, and the
>   single global keyboard-focus rule. A surface never stacks two edge mechanisms.
>
> **Authoritative token values now live in `src/styles/globals.css` `:root` +
> `plugins/builtin/theme/themes/*`.** The frontmatter palette / typography /
> rounded values below are a pre-redesign snapshot retained for historical
> context — where they disagree with globals.css, globals.css wins.

## 0. Design language (the five pillars)

The whole system reduces to five decisions. Everything below elaborates them.

1. **Tool windows around one reading plane** (revised 2026-08; supersedes both
   "flush background delta" and "card over drawer"). Three opaque materials — the
   plane you read on, the chrome columns that frame it, the cards placed on it —
   separated by VALUE, with **a single device pixel** over that step at each seam.
   The plane is the darkest surface on dark and the brightest on light; the chrome
   steps the other way. Both halves are load-bearing: the step alone measured too
   small to read, and a line alone draws the columns as a wireframe of pasted
   rectangles. Which mechanism draws which boundary belongs to the active visual
   style, not to any call site — this one spells its three seam tokens as hairlines;
   a spatial style spells the same three as casts and nothing else changes.
2. **Near-monochrome, one restrained accent** — overall black/white/grey; the
   accent (a calm **blue**, user-selectable) marks live state, progress, focus,
   links, and the one primary action per surface. It is the CTA fill too: this
   language spends its single colour on the action that matters and leaves
   everything else grey.
3. **Dual-theme parity, follows the OS** — light and dark are both first-class
   and polished; the default theme is "system" and tracks `prefers-color-scheme`
   live. The two are mirrors of one region algorithm, not two hand-built skins.
4. **Native system font, mono as the technical voice** — SF Pro / PingFang on
   macOS (the OS UI face) for language; mono for everything that is data — paths,
   counts, durations, tokens, shortcuts, code. That split is what makes a dense
   agent transcript scannable.
5. **Dense, not cramped** — a workbench rhythm: 42px chrome bars, short rows, one
   centred reading measure flanked by navigation rails. Features are first-class
   grouped entries in the sidebar + settings, not buried in the command palette.
   **No tab strip** — one active session; workspace views open full-pane or in the
   dock.

Constant across all of it: the `@theme` token bridge, plugin-contributed chrome,
accent _scarcity_, tabular numerals, keyboard-focus discipline, reduced-motion.

---

## 1. Overview

Lyra is an agent client — a desktop application (Wails / React) that streams Lyra Runtime Protocol events from a Go runtime and renders them as a chat surface with inline tool calls, code, diagrams, and approval flows. The frontend is a **view onto a runtime**, not the runtime itself — but it presents as a refined, calm product surface, not a dense console.

Light and dark are **equal first-class themes**; the default follows the OS (`prefers-color-scheme`) and tracks it live. Neither scheme is second-class.

**Reference** — the direction is the JetBrains tool-window language: an editor you
are *inside*, framed by opaque panels, with the technical layer set in mono.

- **Region model**: three materials, each seam a half-pixel hairline over a value
  step. The reading plane is the one surface that is not chrome.
- **Density**: short chrome bars, two-line index rows, borderless cards.
- **Voice**: sans for language, mono for data — and the mono is load-bearing, not
  decorative, because most of what an agent transcript reports IS data.

**Explicitly rejected** (both prior passes):
- Region hairlines and seam rings (regions separate by value + cast now)
- Cards-on-canvas gutters, panel drop shadows, and glass blur outside floating panels
- An inverting ink CTA that kept the accent unused (the accent IS the CTA)
- A bundled UI webfont where the native OS face reads more premium
- Pill-radius CTAs, ALL-CAPS letter-spaced labels, 700+ display weight
- Bright focus halos/glows that flash on click (focus is a single quiet stroke)

## 2. Color

### Philosophy

Color carries information, not decoration. The system uses **one chromatic accent**, **four greys for surfaces**, **three greys for hairlines**, and **four semantic colors used sparingly**. Decoration comes from the surface ladder, not from color variation.

### Surface anchors

Values live in `themes/lyra-*.ts` and are restated for first paint in
`globals.css`. This table says what each anchor is FOR; it deliberately does not
repeat the hexes, which is how the previous version of it went stale.

| Token | Role |
|---|---|
| `canvas` (`--color-bg`) | The reading plane — transcript, view bodies. Darkest surface on dark, brightest on light. |
| `surface` | Region chrome — the drawer, the dock, the bars that frame the plane. |
| `card` (`--app-card-surface` → `--color-elevated`) | An object placed on a region: a message, a tool card, the composer. |
| `sunken` (`--color-sunken`) | A well cut into a surface: code bodies, terminals, diff hunks, text fields, progress tracks, and inline code in prose. |
| `surface-2` / `-3` / `-4` | Derived chip rungs above `surface` — badges, kbd, selected rows, resting control fills. Mixed out of the CHROME grey, so they belong on chrome; on the plane they read as grime. |

**Why four anchors and not one ladder.** The reading plane is the extreme of its
scheme — pure white on light, near-black on dark — and an object on it steps IN,
toward the chrome. On light that reads as a ladder: every region steps down from
white. On dark it cannot, because the WELL still goes the other way — a card lifts
UP off the plane while a code body recedes BELOW it. One monotonic mix cannot say
both, so `elevated` and `sunken` stay anchors.

A card used to be spelled `#ffffff` over an off-white plane, which is the same
delta pointing the other way — 1.2 L with zero chroma, so the object was held up
entirely by its cast. Stepping in instead gives it a value AND a hue of its own.

**One hue, chroma by area.** Every neutral sits on the accent's hue, and carries
chroma in inverse proportion to the area it covers: the plane none, the chrome
~0.006, a card ~0.008, a well ~0.016 (dark: 0.008 / 0.010 / 0.015). Under roughly
C 0.005 a grey's hue is not addressable in 8-bit sRGB — one byte swings it 20–40° —
so a near-neutral ramp cannot pick its own hue, and a hue nobody chose is what
"dirty grey" means. Chroma is what makes the set read as one material family;
keeping it low on the large areas is what keeps the same decision from reading as
a blue tint.

`surface-2/3/4` derive from `surface` via `color-mix(--depth-step)` so the
contrast preference moves the chip rungs per scheme — they are never pinned
inline. **The step is scheme-aware**: dark doubles it, because 4% of a near-white
ink over a near-black surface moves it a third as far, in perceived lightness, as
4% of a near-black ink over a near-white one.

### Hairlines

A hairline is the edge of a **control** — a text field, a chip, the composer — and
of a **region**: the two are the same primitive at two weights. A change of region
takes `border-soft`, a bar inside one takes `border`, and the reference weights them
apart by the same ratio (207 against 225 on a 255 plane). Regions get theirs from
`--app-card-edge` / `--app-pane-split` / `--app-header-edge`, whose SHAPE the visual
style owns; a callsite never draws a region boundary itself.

Half a pixel, not one. On a 2x panel that is exactly one device pixel, which is what
makes an edge crisp without giving it weight — at 1px the composer carried the
heaviest line on the screen. The earlier revision of this section spread a
directional cast at each seam instead; a cast lands ON the reading plane, so all
three seams read as pressing down on the document.

There is one other case, and only one: an object that **demands attention** — a
pending approval, a failed run. It is neither a region nor a control, and it takes
a 1px `--color-<tone>-edge` over a neutral fill. The fill stays neutral because
these run 200px tall and a wash at that size is a lot of colour for "please look";
the small inline notices are the inverse, and tint instead. This is the only place
the language spends a border on meaning rather than on affordance. The three-step
ramp (`border` / `border-soft` / `divider`) uses literal hex per theme, because a
semi-transparent border shifts across surface lifts and reads as approximate.

**Ink, by contrast, may derive.** Unlike hairlines, the ink ramp (`text-soft` / `text-muted` / `text-faint`) *should* adapt to the surface behind it — that's the Apple label model. A theme can ship just `text` + `text-bright` and let the soft/muted/faint steps derive as `text` at ~82% / ~56% / ~38% alpha over transparent (so they composite against whatever surface they sit on). Palette themes (Solarized, Catppuccin, Tokyo Night, One Dark) instead pin explicit ink hues — their ramp is part of the palette identity, not a single hue at falling opacity. The first-party Lyra themes keep explicit values too; the derivation is the low-friction default for third-party themes.

### Accent policy

The single accent (default `#6c97ff` dark / `#2563eb` light, a calm blue;
user-selectable, with green / pink / orange as alternates) is reserved for
**exactly four surfaces**:

1. Active tab indicator (2px underline on `chat-tab.active`)
2. Primary CTA fill (`button-primary`, Send button)
3. Focus ring (`:focus-visible` — a single thin stroke, **no halo / glow**; one global rule, never drawn at a callsite)
4. Live indicator (streaming dot, running pill, `tab-dot.running`)

Forbidden surfaces for accent: section background, card fill, avatar background, decorative borders, status icons that are not "live". And **no bright accent ring on input focus or click** — inputs/composer strengthen their border quietly instead (the loud halo read as cheap).

"Card fill" here means the accent as a **colour**. The surface anchors sitting on
the accent's *hue* at C ≤ 0.016 is the neutral algorithm above, not accent usage:
at that chroma nothing reads as blue, and the alternative is not a purer neutral
but an unchosen one.

### Semantic palette

| Token | Hex (dark) | Use |
|---|---|---|
| `success` | `#3fb950` | Run finished cleanly, action confirmed. Allowed in: run pill (idle/done), `tab-dot.idle` after success. |
| `warning` | `#f0a936` | User attention required. Allowed in: `approval-card`, `tab-dot.waiting`. |
| `negative` | `#f85149` | Error. Allowed in: `run-error-banner`, tool-call `status: err`. |
| `info` | `#58a6ff` | Information / link. Allowed in: inline links, info badges. |

**Each semantic is two colours, and only one of them is pinned.** The theme
spells the INK — the tone a status word, an icon or a 6px dot is drawn in, whose
luminance is pulled until it clears 4.5:1 on the darkest surface it can sit on.
Every TINT (`-wash` / `-badge` / `-edge`, and the diff row and word tints) mixes
instead from `--tone-*`, the same hue lifted to L68 at 1.4× chroma. One token
cannot do both jobs: light `warning` is pinned at L51 C.10, an olive, and a tint
mixed from an olive reads as dirt rather than as amber. The fill tone is derived,
not shipped, so the hue is still said once — by the theme, or by the user's accent
pick — and no palette has to carry a second set. Anything that must be legible on
its own keeps the ink: these tones are near 1.9:1 on white and would fail 1.4.11
as a mark.

**Semantic colours are scheme-tuned.** The dark values above are desaturated +
lifted in luminance so they don't vibrate or edge-bleed on the near-black canvas
(Apple Dark Mode / Ant dark). Light themes keep the saturated web values
(`#ee0000` / `#0070f3` …) — those read crisp on white. Palette themes
(Catppuccin / Tokyo Night / Solarized / One Dark) ship their own canonical
semantic tones and are left untouched.

## 3. Typography

### Font families

The **native OS font**, no bundled webfont — the system face reads more premium
and native than any shipped font, loads instantly, and renders mixed CJK best:

- **Sans** (`--font-sans`) — SF Pro on macOS (via `-apple-system` / `BlinkMacSystemFont` / `system-ui`), Segoe UI / Roboto elsewhere, **PingFang SC** (+ Hiragino / Microsoft YaHei) for CJK. The primary UI face; display + body share it, weight does the hierarchy.
- **Mono** (`--font-mono`) — SF Mono / Menlo (`ui-monospace`). Used for genuine data only: code, IDs, timestamps, file paths, tool signatures.
- A single `--font-sans` / `--font-mono` token (no `--font-ui` split); the user can override either in Settings → Appearance.

### Scale

The full scale is 11 tokens — narrower than the previous 13-step Spotify scale. Display sizes are smaller than typical marketing systems because Lyra is a product UI, not a hero page.

(See frontmatter `typography:` for canonical sizes / weights / tracking.)

### Principles

1. **Sans-first; mono is the _data_ voice only.** Labels, section headings, nav, speaker names, view titles + subtitles are **sans**. Mono (`caption-mono` / `code`) is reserved for genuine data — IDs, durations, timestamps, file paths, tool-call signatures. (The earlier "mono as eyebrow everywhere" read as an engineering console; pulled back.)

2. **500 is the display ceiling.** Never use 600+ for UI (700, 800, 900 right
   out). Hierarchy comes from size + weight contrast (400 vs 500), not from
   going heavier. (600 semibold is reserved for HITL action buttons only.)

3. **Display gets negative tracking.** -0.96px at 32px display-xl, scaling proportionally. Body holds at -0.05 to 0.

4. **Sentence-case headlines.** Never ALL-CAPS. Welcome screen, settings sections, view headers — all sentence-case. Optional period termination is allowed (Vercel signature) but not required.

5. **Tabular numerals everywhere numeric.** `font-feature-settings: "tnum"` on caption-mono and code by default. Numbers don't jitter when counters update.

6. **CJK safety.** Letter-spacing > 0.02em should be scoped to `:lang(en)` — CJK characters are pulled visually apart by positive tracking.

## 4. Layout

### App shell

Three opaque tool windows and no line between any of them. No bottom status bar:

```
 drawer (fixed, slides)      reading plane (z-15)                    dock
┌─────────────┐┌──────────────────────────────────────────────┬─────────┐
│ 42px header ││ 42px header: project / title · state · meta   │ 42px    │
│ project     │├──────────────────────────────────────────────┤ tabs +  │
│ ⌘N ⌘K …     ││ ·                                       In   │─────────│
│ Projects    ││ ·      Message stream (--content-max)   this │         │
│   session   ││ ·                                       answ │  views  │
│   session   ││                                              │         │
│             ││    ┌──────────────────────────────────────┐  │         │
│ ⚙ settings  ││    │ Composer                             │  │         │
└─────────────┘└────┴──────────────────────────────────────┴──┴─────────┘
       ↑          ↑ turn rail (44)          outline rail (186) ↑ dock casts
   --sidebar-width  --app-card-edge: the drawer's cast,          leftward
   (240, resizable) drawn inside the plane                    (--app-pane-split)
```

Both rails are container-query gated on the width of the reading column — not the
window — because the drawer and the dock change it without the window changing at
all. Banners and composer take the same gutters, so the three stay on one axis.

### Sidebar

- **Default state: expanded** (`--sidebar-width`, 240px, user-resizable by dragging
  the seam rail; floor 208px, and the reading column never goes below 640px).
- **Pinned identity** above the scrolling index: the active session's workspace,
  because the one fact you must be able to read without scrolling is where the next
  command will run.
- **Two-line session rows**: title, then state and time — the index is something you
  triage from, not just a list of names.
- **Collapsed** (`⌘B`) slides the drawer fully off-canvas under the card — there is
  no icon rail. The card then reaches the window edge, squares its seam corner, and
  its header widens its leading inset to clear the macOS traffic lights.
- **One visible collapse control.** Expanded, the toggle lives in the drawer's
  46px header after the traffic-light gutter; collapsed, ownership moves to the
  content header. Keyboard focus follows that handoff instead of falling onto the
  document.
- The seam rail is a focusable vertical separator: pointer movement writes only
  `--sidebar-width`, pointer release commits once, and Arrow/Home/End provide the
  same bounded resize path for keyboard users.
- The drawer is opaque region chrome (`--app-drawer-surface`), never a translucent
  sheet. It carries no border and casts no shadow of its own: the plane draws the
  seam as an inset cast, because the plane outranks the drawer on z-index so the
  drawer can slide underneath it.

### Chat measure

- Message stream + composer both cap at **`--content-max`**, centered between the
  rails, with a `--density-column-gutter` inset.
- A turn is a caption line over a full-width body, not an avatar gutter beside a
  narrowed one: who is speaking is read once, the measure is inhabited for the
  whole turn, and a 38px gutter was taking it from every code block and table.
- Long code blocks and tables can exceed the measure — they scroll horizontally
  inside their own wrapper; the prose column does not move.

### Tabs — removed (2026-06 redesign)

- **No tab strip.** One active session at a time (ChatGPT-style); switching is
  via the sidebar session list (`selectTab`). Workspace views (Files / Diff /
  Plan / Tools / …) open **full-pane** (no tab affordance, no title bar) and
  close via **Option A**: click the same sidebar nav row again (toggle), press
  `Esc` (yields to palette/dialog/input first), or use the split-view
  promote/close control.

### Spacing rhythm

Lyra is a **product UI**, not a marketing site. Spacing values from the frontmatter `spacing:` block apply, but:

- Section breaks inside a panel: `md` 16px to `lg` 24px (never `5xl` 96px — too marketing).
- Card interior padding: `md` 16px default, `lg` 24px for emphasized cards.
- Inline gaps: `xs` 8px to `sm` 12px.
- Marketing-band spacing (`5xl` / `section` 192px from Vercel) is **not used** in Lyra except in the welcome screen.

## 5. Elevation & Depth

**Depth is value plus a directional cast. Flush chrome casts nothing.**
Every seam between regions is carried by a short, tight cast from the panel that
overlaps — `--app-card-edge` at the drawer seam (drawn INSIDE the plane, because
the plane outranks the drawer on z-index so the drawer can slide under it),
`--app-pane-split` where the dock meets the conversation. No region anywhere
carries a border. The only elements with a real drop shadow are **truly-floating
overlays** (menus, popovers, tooltips, command palette, lightbox), which have no
value delta to lean on because they can land over anything.

| Level | Treatment | Use |
|---|---|---|
| 0 | Region fill only | The reading plane, prose, a message body |
| 1 | `bg-card` | Message card, tool card, composer, plan card, table |
| 2 | `bg-sunken` | Code body, terminal, diff hunk, text field, progress track |
| 3 | `surface-2` / `-3` | Chips, badges, kbd, selected rows — on chrome, not on the plane |
| 4 | `--shadow-popover` | Floating overlays only — one token, ring plus depth |

Each role owns exactly ONE edge mechanism. A border and a shadow ring on the same
surface is a double edge; two 1px semi-transparent lines sharing a pixel double
their alpha and read as a bright dot.

**Row state is not on this ladder.** Hover and selection are `bg-hover` /
`bg-selected` — an ink wash (`--color-hover` / `--color-selected`), so a row
lights up the same over a card, a menu or the drawer, and selection stays legible
while the pointer sits on its neighbour. A surface step as a hover paints a slab
where there was none; `check-interactive-chrome` fails the build on both that and
a hand-picked `hover:bg-fg/[…]` alpha.

This holds identically in **both schemes**; only the cast's strength differs, and
that is one palette value (`--shadow-cast`) rather than a per-component decision.
(Both earlier models are gone: cards-on-canvas with gutters and multi-layer drops,
and the 2026-07 seam-ring pass that gave every boundary a hairline.)

## 6. Shapes

### Radius scale

Four values do all the work. The visual style owns them (`style-shape-*`); the
user's radius preference multiplies through.

| Token | Value | Use |
|---|---|---|
| `none` | 0px | Full-bleed bars |
| `xs` | 4px | Anything that is really a tag — badges, inline code |
| `sm` | 6px | Controls: buttons, chips, index rows, dock tabs |
| `md` | 8px | Cards, text fields, segmented tracks |
| `lg` | 10px | Surfaces that float or receive typing: composer, popovers |
| `xl` | 12px | Lightbox frame |
| `pill` | 9999px | Status dots, progress tracks, avatars ONLY |

### NEVER

- **No pill-radius CTAs.** The action button is a rounded square on the control
  ladder — a lone disc beside a row of rectangles reads as a different kit.
- **No mixed scales on one screen.** One corner language, four steps, no exceptions.

## 7. Motion

(See frontmatter `motion:` for tokens.)

- **Hover / press feedback**: `dur-fast` 140ms with `ease-out`. Every interactive element has a visible state change.
- **Layout enter/exit** (modal, toast, palette): `dur-med` 220ms with `ease-emphasized`.
- **Heavy transitions** (panel slide, accordion expand): `dur-slow` 360ms with `ease-emphasized`.
- **Active press scale**: `active:scale-[var(--press-scale)]`. One value for the
  whole app — a control that sinks 0.90 next to one that sinks 0.98 reads as two
  different apps. Per-element amounts were tried and drifted to four of them.
- **`prefers-reduced-motion`**: all transitions degrade to ≤80ms, all scale animations disabled.

## 8. Components

The frontmatter `components:` block carries the canonical spec for every Lyra-specific component. Highlights:

### Tool-call card — the "RPC log" rule

Tool calls render **like an RPC log entry, not a generic card**:

```
read_file(path: "src/auth.ts")            ✓ 12ms · 1,247 lines
```

- Line 1: function signature in `code` typography (mono 12.5px).
- Line 2: status glyph (`✓` / `✗` / pulsing dot) + duration + summary, all `caption-mono`.
- Expandable for full result body.
- Card chrome: `surface-1` + `hairline` + `md` radius.

This single change carries more "agent-tool" feel than any other component decision.

### No bottom status bar

There is no dense bottom data row. Run telemetry (tokens / cost / rate) lives in the **composer footer**; global status + notifications live in the **sidebar footer**. A persistent mono data strip read as "console" — the chrome stays calm.

### Reasoning block — mono header, no caps

Header was `THOUGHT FOR 1S` ALL-CAPS — now `thought · 1.2s` in `caption-mono` lowercase. Body italic stays.

### Composer cheatsheet — auto-derived

The composer's hover-revealed cheatsheet **derives rows from `useCommands()`**. Any command with a `shortcut: string` field auto-appears. Static rows reserved for composer-local keys (`Enter`, `⇧↵`, `⌘↵`, `Esc`, `⌘1-9`).

## 9. Accent Usage Policy (strict)

Accent (default `#6c97ff` dark / `#2563eb` light, user-selectable) appears in:

1. **Active tab indicator** — 2px underline on `chat-tab.active::after`
2. **Primary CTA fill** — `button-primary` background, send button
3. **Focus ring** — `:focus-visible`, a single thin accent stroke (**no halo / glow**, and never on plain mouse-focus of inputs). One global rule in globals.css draws it for everything; mark `data-focus-inset` where it would land outside the box, `data-chrome-focus` where a row fills instead. A theme retunes it through `--color-focus-ring`. Never drawn at a callsite — `check-interactive-chrome` fails the build
4. **Live indicator** — streaming `tab-dot.running`, status pill while `run.running === true`, the reasoning block's pulse dot

That's the entire list. Accent does **not** appear in:
- Avatar backgrounds (use `surface-2/3` + `ink-muted`)
- Section headers (use `ink`)
- Active-state list rows (use `surface-2/3` + `ink`)
- Iconography (icons are `ink-muted` → `ink` on hover)
- Tool-call success status (use `success`, not the accent)
- Input / composer focus (a quiet border strengthen — no accent ring)

When in doubt: **does this surface convey "the agent is alive and live"?** If yes, accent. If no, grey.

## 10. Do's and Don'ts

### Do

- Render IDs / durations / file paths / tool signatures in mono; labels, headings + names in **sans**.
- Cap chat content (message stream + composer) at `chat-measure: 760px`, centered.
- Use literal hex hairlines — not `color-mix(text X%, transparent)`.
- Set every interactive element with `:hover`, `:active`, `:focus-visible`.
- Use `font-feature-settings: "tnum"` on every numeric display.
- Default sidebar to **expanded** (248px); collapse to the rail (56px) on demand (⌘B).
- Render tool calls as RPC logs (mono signature + duration line — the one place mono stays).
- Pair display weight 600 with body weight 400. Hierarchy via size + weight contrast, never weight 700+.

### Don't

- **Don't use ALL-CAPS labels with letter-spacing.** Section labels / eyebrows / table heads are **sentence-case** (mono for dense technical labels like `args` / `attrs`); the ALL-CAPS + wide-tracking eyebrow is the rejected Sonance vocabulary.
- **Don't use pill-radius CTAs** (`9999px`, `500px`, `100px` on a button). Buttons are `md` 8px.
- **Don't use weight 700+ for display.** 600 is the ceiling, Linear and Vercel both forbid this.
- **Don't add panel / card drop shadows.** The layout is flush — depth is the surface step + hairlines. Stacked-subtle shadow is for truly-floating overlays (Level 4) only, in BOTH schemes. No cards-on-canvas, no gutters.
- **Don't use pure `#000000` or a harsh near-black canvas.** Dark canvas is `#0c0d0f` — a soft, comfortable dark for a full reading surface.
- **Don't flash a bright accent ring/halo on focus or click.** Keyboard focus is one thin stroke; inputs/composer just strengthen their border. The loud glow read as cheap.
- **Don't introduce a second chromatic accent.** Lyra has one accent + four semantic colors. No more.
- **Don't use accent decoratively.** Active tab / primary CTA / focus ring / live indicator — that's the entire allowed list.
- **Don't set body paragraphs in mono.** Mono is for the technical layer only.
- **Don't apply atmospheric gradients, mesh backdrops, or dot grids** (the latter was discussed and rejected — Linear explicitly forbids "atmospheric gradients or spotlight cards").
- **Don't add backdrop-filter / vibrancy / Mica effects.** Wails WebView is inconsistent across platforms; visual carries the load.

## 11. Light theme

Light is full parity, not second-class — and the **default theme follows the OS**
(`prefers-color-scheme`, live). It runs the same region algorithm mirrored: the
plane is the brightest surface, the chrome steps down from it, cards lift to
white, wells recede. Values live in `themes/lyra-light.ts`.

Two places where light is not a mechanical inversion, both for the same reason —
ink cannot be:

- **Semantic hues sit one step deeper** than the reference language's. Its greens
  and ambers land at 3.4–3.9:1 as text on this chrome, and a status word nobody
  can read is not a status. Hue family preserved, luminance pulled until each
  clears 4.5:1 on the darkest surface it can sit on.
- **The accent is the deeper blue.** Same reason; the accent carries link text.

## 12. References

- **JetBrains tool windows** — the region model: an editor you are inside, framed
  by opaque panels, separated by value rather than by lines.
- **Linear-app** — the scarce single-accent policy and sentence-case labels.
- Lyra Runtime Protocol — `frontend/src/protocol/run/` + `frontend/src/rpc/` — drives the shape of the data this UI renders.

## 13. Iteration guide

1. When adding a new surface, reference its component spec in the frontmatter `components:` block. If none exists, propose one (commit + this doc together).
2. Verify BOTH schemes (the default follows the OS) before merging visual changes.
3. Run `npx tsc --noEmit && npx vitest run` after any token change.
4. Visually verify in `wails3 dev` — type/spacing changes especially.
5. Treat the accent as scarce: ask "is this live?" — if no, use grey.
