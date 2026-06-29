---
version: 1.0
name: lyra-design-analysis
description: "Lyra is an agent client — a desktop chat shell that streams Lyra Runtime Protocol events from a Go runtime. The design is calm, airy, premium-minimal (the Codex / Linear-app reference): a flush edge-to-edge layout (no cards-on-canvas gutters), a near-monochrome palette with one restrained blue accent, full light/dark parity that follows the OS by default, the native system font (SF Pro / PingFang on macOS), and sans-first labels (mono reserved for code / IDs / paths). Built for engineers reading streaming markdown, inspecting tool calls, and approving actions — but reading as a refined product surface, not a dense console. (Replaced an earlier dense, dark-first, green-accent, Geist-on-cards-on-canvas system.)"

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
    description: Textarea + toolbar surface. Anchored bottom, centered to chat-measure. Focus ring uses primary at 14% alpha + hairline-strong.
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
  # workspace views open full-pane. See REDESIGN.md §5 Step 2a.
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

> **Redesign 2026-06 (landed on `main`).** The OpenAI-restrained redesign
> — see [`REDESIGN.md`](./REDESIGN.md) for the full step-by-step + commit log —
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
> - Shadow ladder精简 to 3 canonical tokens (`composer`/`elevated`/`focus`).
>
> **Authoritative token values now live in `src/styles/globals.css` `:root` +
> `plugins/builtin/theme/themes/*`.** The frontmatter palette / typography /
> rounded values below are a pre-redesign snapshot retained for historical
> context — where they disagree with globals.css, globals.css wins.

## 0. Design language (the five pillars)

The whole system reduces to five decisions. Everything below elaborates them.

1. **Flush, edge-to-edge layout** — no cards-on-canvas. The sidebar + main run to
   the window edge, separated by a **background delta** (`surface` vs `canvas`),
   not a hairline. The sidebar is the one half-step-lifted surface; the main
   area paints on the canvas. No gutters, no panel drop shadows (dark _or_ light).
2. **Near-monochrome, one restrained accent** — overall black/white/grey; the
   accent (a calm **blue**, `#7b8efa` dark / `#2563eb` light by default,
   user-selectable) appears only on live / steer / focus, and focus is a single
   thin stroke — never a bright halo or glow on click. The primary CTA fill is
   `bg-fg` (black/white), NOT accent — accent is reserved for "live" state.
3. **Dual-theme parity, follows the OS** — light and dark are both first-class
   and polished; the default theme is "system" and tracks `prefers-color-scheme`
   live.
4. **Native system font** — SF Pro / PingFang on macOS (the OS UI face), no
   bundled webfont. Sans-first labels everywhere; mono is reserved for genuine
   data (code / IDs / timestamps / paths), not every eyebrow.
5. **Airy + discoverable** — generous whitespace, calm rhythm; features are
   first-class grouped entries in the sidebar + settings (not buried in the
   command palette). **No tab strip** — one active session; workspace views
   open full-pane and close via sidebar-toggle / `Esc`.

Constant across all of it: the `@theme` token bridge, plugin-contributed chrome,
accent _scarcity_, tabular numerals, keyboard-focus discipline, reduced-motion.

---

## 1. Overview

Lyra is an agent client — a desktop application (Wails / React) that streams Lyra Runtime Protocol events from a Go runtime and renders them as a chat surface with inline tool calls, code, diagrams, and approval flows. The frontend is a **view onto a runtime**, not the runtime itself — but it presents as a refined, calm product surface, not a dense console.

Light and dark are **equal first-class themes**; the default follows the OS (`prefers-color-scheme`) and tracks it live. Neither scheme is second-class.

**Reference** — the calm, airy, premium-minimal direction draws on OpenAI's Codex desktop app and the Linear-app aesthetic:

- **From Codex**: the flush edge-to-edge layout, the calm centered empty-state (hero + composer as one group), generous whitespace, and grouped, discoverable navigation.
- **From Linear**: the scarce single-accent policy, the surface-step + hairline approach to depth (no decorative shadow), and sentence-case labels.

**Explicitly rejected** (the prior dense / dark-first / "Spotify-derived" vocabulary, _and_ the interim Linear+Vercel-synthesis pass it replaced):
- Cards-on-canvas gutters + panel drop shadows (the layout is flush)
- A green brand accent + mono-as-eyebrow-everywhere (accent is a restrained blue; mono is data-only)
- A bundled webfont (Geist) where the native OS face reads more premium
- Pill-radius CTAs, ALL-CAPS letter-spaced labels, 700+ display weight, accent used decoratively
- Bright focus halos/glows that flash on click (focus is a single quiet stroke)

## 2. Color

### Philosophy

Color carries information, not decoration. The system uses **one chromatic accent**, **four greys for surfaces**, **three greys for hairlines**, and **four semantic colors used sparingly**. Decoration comes from the surface ladder, not from color variation.

### Surface ladder

| Token | Hex (dark / light) | Use |
|---|---|---|
| `canvas` | `#0c0d0f` / `#ffffff` | **The main reading area** in the flush layout — chat stream, view bodies. The sidebar sits a half-step above it. |
| `surface` | `#16181b` / `#f6f7f8` | The one lifted chrome step — sidebar, cards, message bubble, tool-call card. Divided from canvas by a hairline, not a gutter. |
| `surface-2` | derived | Hovered / active row, raised surface, command palette, **inset reading pane** (settings content area — distinct from the outer chrome). |
| `surface-3` | derived | Sub-nav, dropdown, popover. |
| `surface-4` | derived (rare) | Deepest lifted surface. |

`surface-2/3/4` derive from `surface` via `color-mix(--depth-step)` so the global contrast slider moves the whole ladder per scheme — they're never pinned inline.

**Inset-pane rule** — when a workspace view has its own internal nav rail + content split (e.g. the settings page), keep the rail on the canvas/chrome and lift the content area to `surface-2` so the reading zone reads as the focus.

### Hairlines

| Token | Hex | Use |
|---|---|---|
| `hairline` | `#23252a` | Default 1px border on cards, dividers, table rows. |
| `hairline-strong` | `#34343a` | Input focus border, emphasized divider. |
| `hairline-tertiary` | `#3e3e44` | Borders on nested surfaces. |

**Hairlines must use literal hex values, not `color-mix(text X%, transparent)`** — semi-transparent borders shift visually across different surface lifts and read as "approximate". Literal hex = precision = perceived craft.

**Ink, by contrast, may derive.** Unlike hairlines, the ink ramp (`text-soft` / `text-muted` / `text-faint`) *should* adapt to the surface behind it — that's the Apple label model. A theme can ship just `text` + `text-bright` and let the soft/muted/faint steps derive as `text` at ~82% / ~56% / ~38% alpha over transparent (so they composite against whatever surface they sit on). Palette themes (Solarized, Catppuccin, Tokyo Night, One Dark) instead pin explicit ink hues — their ramp is part of the palette identity, not a single hue at falling opacity. The first-party Lyra themes keep explicit values too; the derivation is the low-friction default for third-party themes.

### Accent policy

The single accent (default `#6c97ff` dark / `#2563eb` light, a calm blue;
user-selectable, with green / pink / orange as alternates) is reserved for
**exactly four surfaces**:

1. Active tab indicator (2px underline on `chat-tab.active`)
2. Primary CTA fill (`button-primary`, Send button)
3. Focus ring (`:focus-visible` — a single thin stroke, **no halo / glow**)
4. Live indicator (streaming dot, running pill, `tab-dot.running`)

Forbidden surfaces for accent: section background, card fill, avatar background, decorative borders, status icons that are not "live". And **no bright accent ring on input focus or click** — inputs/composer strengthen their border quietly instead (the loud halo read as cheap).

### Semantic palette

| Token | Hex (dark) | Use |
|---|---|---|
| `success` | `#3fb950` | Run finished cleanly, action confirmed. Allowed in: run pill (idle/done), `tab-dot.idle` after success. |
| `warning` | `#f0a936` | User attention required. Allowed in: `approval-card`, `tab-dot.waiting`. |
| `negative` | `#f85149` | Error. Allowed in: `run-error-banner`, tool-call `status: err`. |
| `info` | `#58a6ff` | Information / link. Allowed in: inline links, info badges. |

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

Flush, edge-to-edge — a **background delta** divides the sidebar from the main
area (`surface` vs `canvas`, no hairline); no gutters, no bottom status bar:

```
┌────────────┬────────────────────────────────────────────────┐
│ Sidebar    │ (drag region — thin strip, no tab strip)       │ 36px
│ 248        │────────────────────────────────────────────────│
│  · new     │                                                 │
│  · search  │           Message stream (max-width 720)        │ 1fr
│  Workspace │                                                 │
│  Projects  │  ┌──────────────────────────────────────────┐   │
│  …         │  │  Composer (max-width 720, rounded-xl)    │   │
│  ⚙ settings │  └──────────────────────────────────────────┘   │
└────────────┴────────────────────────────────────────────────┘
   ↑ surface (chrome)        ↑ canvas (main reading area)
   separated by background delta — no gap, no line, no shadow
```

### Sidebar

- **Default state: expanded** (248px). The primary-nav group (New chat / Search + a "Workspace" group of feature destinations) and the project/session tree are the navigation, visible up front. Features that used to hide behind the palette get first-class grouped entries here.
- **Rail** (56px, icon-only) is the on-demand collapsed mode via `⌘B`.
- The sidebar is the one lifted `surface`; the main area is `canvas`. They're flush, separated by a **background delta** (no `border-right`).

### Chat measure

- Message stream + composer both cap at **`--content-max: 720px`**, centered.
- Reading ergonomics: 45–75 characters per line at body-md (14px) translates to ≈ 720–820px. 760 is the comfortable middle.
- Long code blocks and tables can exceed 760 — they get horizontal scroll inside their own wrapper, the prose column stays at 760.

### Tabs — removed (2026-06 redesign)

- **No tab strip.** One active session at a time (ChatGPT-style); switching is
  via the sidebar session list (`selectTab`). Workspace views (Files / Diff /
  Plan / Tools / …) open **full-pane** (no tab affordance, no title bar) and
  close via **Option A**: click the same sidebar nav row again (toggle), press
  `Esc` (yields to palette/dialog/input first), or use the split-view
  promote/close control. See REDESIGN.md §5 Step 2a for the full rationale +
  the `sessionStore` deprecation of `tabIds` / `mainViewTabs`.

### Spacing rhythm

Lyra is a **product UI**, not a marketing site. Spacing values from the frontmatter `spacing:` block apply, but:

- Section breaks inside a panel: `md` 16px to `lg` 24px (never `5xl` 96px — too marketing).
- Card interior padding: `md` 16px default, `lg` 24px for emphasized cards.
- Inline gaps: `xs` 8px to `sm` 12px.
- Marketing-band spacing (`5xl` / `section` 192px from Vercel) is **not used** in Lyra except in the welcome screen.

## 5. Elevation & Depth

**Flush, so depth is the surface step + hairlines — not gutters or panel shadows.**
The layout has no cards-on-canvas frame: the sidebar (`surface`) meets the main
area (`canvas`) at a single hairline, and neither casts a shadow (dark _or_
light). The only elements that get a real shadow are **truly-floating overlays**
(command palette, mermaid lightbox, toaster) — a stacked-subtle drop, never a
single 24px blur. Everything else builds depth from the ladder:

| Level | Treatment | Use |
|---|---|---|
| 0 | No border, no shadow | Body text, message body, the main area |
| 1 | `surface` background + 1px `hairline` border | Default card, tool-call card, message bubble, sidebar |
| 2 | `surface-2` background | Active / hovered row, raised surface, inset reading pane |
| 3 | `surface-3` background | Sub-nav, dropdown |
| 4 | Stacked-subtle shadow + inset hairline | The few truly-floating overlays only |

This holds identically in **both schemes** — light also drops panel/card shadows
(flush), reserving stacked shadows for floating overlays. (The earlier
cards-on-canvas model — floating panels with 8px gutters + multi-layer drop
shadows, and light's full 5-level shadow ladder — is gone.)

## 6. Shapes

### Radius scale

| Token | Value | Use |
|---|---|---|
| `none` | 0px | Full-bleed bars (topbar) |
| `xs` | 4px | Badges, status pills, file chips |
| `sm` | 6px | Inputs, small buttons, icon button square |
| `md` | 8px | **Default button**, card chrome, dialog corners |
| `lg` | 12px | Workspace cards, command palette frame |
| `xl` | 16px | Lightbox frame, hero panels |
| `pill` | 9999px | Status badges, segmented toggle ONLY |
| `circle` | 50% | Avatars, dot indicators |

### NEVER

- **No pill-radius CTAs.** The previous `--radius-pill: 500px` and `--radius-full: 9999px` are removed from button surfaces. Buttons are `md` 8px square.
- **No mixed scales on one screen.** Vercel allows pill at marketing scale + 6px at nav; Lyra picks one scale (8px) and stays there.

## 7. Motion

(See frontmatter `motion:` for tokens.)

- **Hover / press feedback**: `dur-fast` 140ms with `ease-out`. Every interactive element has a visible state change.
- **Layout enter/exit** (modal, toast, palette): `dur-med` 220ms with `ease-emphasized`.
- **Heavy transitions** (panel slide, accordion expand): `dur-slow` 360ms with `ease-emphasized`.
- **Active press scale**: every clickable surface gets `:active { transform: scale(0.92-0.96) }` per element size — gives tactile confirmation.
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
3. **Focus ring** — `:focus-visible`, a single thin accent stroke (**no halo / glow**, and never on plain mouse-focus of inputs)
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

Light is full parity, not second-class — and the **default theme follows the OS** (`prefers-color-scheme`, live). Surfaces: canvas `#ffffff` (the white main reading area), surface `#f6f7f8` (gray chrome). Hairlines `#ebebeb` / `#d4d4d6`. Ink `#171717` / `#4d4d4d` / `#5e5e5e`. The accent is the blue's light variant `#2563eb`; the CTA stays black-on-white.

Light is **flush too** — no card/panel shadow ladder. Like dark, only truly-floating overlays get a stacked-subtle shadow. The two schemes are symmetric.

## 12. References

- **Codex desktop (OpenAI)** — the flush layout, the calm centered empty-state (hero + composer as one group), generous whitespace, grouped discoverable nav.
- **Linear-app** — the scarce single-accent policy, surface-step + hairline depth (no decorative shadow), sentence-case labels.
- Lyra Runtime Protocol — `frontend/src/protocol/run/` + `frontend/src/rpc/` — drives the shape of the data this UI renders.

## 13. Iteration guide

1. When adding a new surface, reference its component spec in the frontmatter `components:` block. If none exists, propose one (commit + this doc together).
2. Verify BOTH schemes (the default follows the OS) before merging visual changes.
3. Run `npx tsc --noEmit && npx vitest run` after any token change.
4. Visually verify in `wails dev` — type/spacing changes especially.
5. Treat the accent as scarce: ask "is this live?" — if no, use grey.
