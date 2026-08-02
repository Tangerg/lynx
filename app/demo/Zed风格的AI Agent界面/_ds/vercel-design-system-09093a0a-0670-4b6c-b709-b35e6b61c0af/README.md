# Vercel Design System

A design system extracted from Vercel's marketing surface and Geist token system. Vercel is a developer-platform brand — their surfaces read as a deployment dashboard's marketing layer, written for engineers who already know the syntax.

> **Sources.** This system was built from Vercel's written brand specification plus the **official Geist token spec** (Light theme, alpha) the token layer is now aligned to — colors (gray + gray-alpha + blue/red/amber/green/teal/purple/pink 100→1000 with Display-P3 variants), the heading/label/copy/button type scale, the 4px spacing scale, the 6/12/16/full radius scale, official elevation, motion easing, and focus ring. `colors_and_type.css` is the source of truth. If you have Vercel's public Geist UI library (https://geist-ui.dev) or the Next.js / Vercel marketing repo, drop it in.

---

## What this system covers

Vercel runs two surfaces with one voice:

1. **Marketing site** (`vercel.com`) — hero bands, pricing, templates, feature grids. White / near-white canvas, ink-near-black text, one signature mesh gradient.
2. **Dashboard / in-app** (`vercel.com/dashboard`) — deployments, project lists, logs, settings. Same tokens, tighter density, the 6 px `--geist-radius` instead of the 100 px marketing pill.

Both share the same Geist type, the same gray scale, the same elevation ladder. The brand reads as one system that flexes between density modes — never as "marketing" and "product" with separate visual languages.

---

## Components

Exported, reusable Geist primitives live in `components/` (each has a `.jsx`, a `.d.ts`, and a `@dsCard` preview). Import via `const { Button } = window.VercelDesignSystem_09093a`:

- **Button** — in-product control. `variant`: primary (gray-1000) · secondary (hairline) · tertiary (ghost) · error (red-800); `size`: sm 32 · md 40 · lg 48; 6 px radius.
- **Input** — text field with the two-layer blue focus ring; `size` sm/md/lg, 6 px radius, hairline border at rest.
- **Badge** — status pill; `tone` maps to the accent scales (100 background + 900 text): gray · blue · amber · red · green · purple.

Larger surface recreations (nav, hero, pricing, dashboard sidebar, deployments table) live in `ui_kits/` as standalone prototypes.

---

## Index

| File / folder | What's in it |
|---|---|
| `README.md` | This file. Overview, content fundamentals, visual foundations, iconography. |
| `SKILL.md` | Agent-skill manifest so this folder is portable into Claude Code. |
| `colors_and_type.css` | All CSS variables (color, type, spacing, radius, shadow) + utility classes for type and component primitives. Import this from any HTML artifact. |
| `assets/` | Vercel triangle mark, wordmark, gradient backdrops, customer-logo placeholders. |
| `preview/` | The Design System tab cards — one tiny HTML page per token concept (Type, Colors, Spacing, Components, Brand). |
| `ui_kits/marketing/` | Hi-fi recreation of the marketing surface: nav, hero, feature grid, pricing, footer. |
| `ui_kits/dashboard/` | Hi-fi recreation of the in-app surface: sidebar, deployments table, project cards, settings. |

---

## Content fundamentals

**Voice.** Engineer-to-engineer. The brand assumes you already know what a deployment is, what a preview URL is, what edge functions do. Copy never explains a primitive — it puts the primitive on screen (a terminal mockup, a code block) and lets it speak for itself.

**Tone.** Direct, declarative, technically literate. Never marketing-glossy. The site sells deployment infrastructure to people who write `git push` for a living and it sounds like it knows that.

**Casing.** Sentence-case everywhere — headlines, button labels, badge text. The ONLY uppercase moments are mono-set eyebrows (`{typography.caption-mono}`) that get rendered with `text-transform: uppercase` to read as terminal-style section markers ("INFRASTRUCTURE", "PLATFORM"). Display headlines are sentence-case and frequently period-terminated:

  * "Build and deploy on the AI Cloud."
  * "Your frontend, delivered."
  * "A compute model for all workloads."

That period is part of the voice. It closes the thought; it makes the headline read as a statement, not a slogan.

**Pronouns.** "Your" carries weight ("Your frontend, delivered."). "We" is rare; the brand prefers naming the product ("Vercel delivers…") to first-person plural. Second-person "you" appears in tutorial copy and CTAs ("Deploy your first app"), never in marketing-aspirational mode.

**Negative tracking is part of the voice.** Display sizes use −2.4 to −0.6 px letter-spacing. Reverting to default tracking *breaks the brand*.

**Mono for the technical layer only.** Section eyebrows, code blocks, terminal mockups, filenames. Body paragraphs *never* set in mono.

**Density.** Marketing copy is short. A feature card is typically: 1 short eyebrow, 1 headline ≤ 6 words, 1 body line ≤ 18 words, 1 CTA. The brand never writes long paragraphs in marketing surfaces.

**No emoji.** Vercel's marketing surface and in-app product UI do not use emoji. Icons are flat monochrome SVG (Lucide-style stroke, 1.5 px stroke weight at 16–24 px). The only "icon" most CTAs carry is a thin arrow glyph (→).

**No exclamation marks.** Confidence is conveyed through the period, not the exclamation. "Build and deploy on the AI Cloud." not "Build and deploy on the AI Cloud!"

**Examples of brand-correct copy:**

  * Eyebrow (mono uppercase): `INFRASTRUCTURE`
  * Headline: "Ship a faster site by Friday."
  * Body: "Preview every commit, ship to the edge in one push, and roll back from a single dashboard."
  * Primary CTA: "Start Deploying"
  * Secondary CTA: "Get a demo"
  * Footer column label: `PRODUCT` (mono caps)

**Examples of brand-INCORRECT copy:**

  * "BUILD AND DEPLOY ON THE AI CLOUD!" (all-caps + exclamation)
  * "🚀 Deploy faster than ever before!" (emoji + hype)
  * "We're so excited to help you ship your next big idea" (chatty, first-person plural, aspirational)

---

## Visual foundations

**Color.** The brand operates on **ink + gray + one mesh gradient**. There is no sixth accent color. Marketing surfaces use only the `100`, `1000`, and `700` levels of the gray scale; the full 100→1000 scale + the blue/red/amber/green/teal/purple/pink scales exist as tokens for in-product surfaces but rarely surface in marketing chrome. Inline links are the one non-grayscale chrome color — `--link` = `--blue-700` `#006bff` (Geist's blue is success, links, and focus in one).

**Type.** Two faces: **Geist** (custom geometric sans, w400/500/600) for everything narrative, **Geist Mono** for the technical layer. Weight 600 is the display ceiling — the geometric sans never appears at 700/800. Display sizes are tracked aggressively negative.

**Spacing.** 4 px base unit. Every captured value in the system is a multiple of 4. Marketing bands use 64–96 px top/bottom; hero bands stretch to 192 px to give the gradient room to breathe. Inside a card the type stack is tight (8 px) but the gap to the CTA cluster is wide. *The page reads as engineered — large gaps + tight interior, never the other way around.*

**Backgrounds.** Four-step surface ladder: `canvas` (`#ffffff`), `canvas-soft` (`#fafafa`), `canvas-soft-2` (`#f5f5f5`), `ink` (`#171717` — the polarity-flipped dark band). The brand *never* uses a background image at large scale beyond the mesh gradient. No noise, no grain, no pattern. The gradient sits on a flat surface; everything else is solid fill.

**The mesh gradient is the only decoration.** Six radial stops — blue → teal → violet → magenta → coral → amber — composited on a white canvas. Used at hero scale only; never miniaturized to a swatch, never cropped to a single color, never reordered. Treat it as one unified atmospheric object.

**Animation.** Subtle. Brand-correct hover transitions are 150 ms ease, color-only. CTAs do *not* bounce, scale, or rotate. The most expressive motion on the site is a 200 ms fade-in for revealed content. Press states use a 0.5 px translateY (the button "settles" half a pixel) — no scale-down, no shadow-flatten. No bounces, no springs, no Lottie illustrations.

**Hover states.**
  * `button-primary` (black pill): background goes from `#171717` to pure `#000`.
  * `button-secondary` (white pill): background goes from `#fff` to `--canvas-soft` `#fafafa`.
  * `nav-link`: text fades from `--body` `#4d4d4d` to `--ink` `#171717` over 150 ms. The ghost-pill background appears only on hover or active state.
  * `link-inline`: underline appears on hover; color does not change.
  * Cards do not have visible hover states on marketing surfaces. In-app cards get a 1 px brighter ring shift.

**Press states.** 0.5 px downward translate. No background change beyond hover. Buttons feel "settled" not "punched."

**Borders.** 1 px hairline (`--hairline` `#ebebeb`) almost everywhere a border appears. Form inputs at rest, table row dividers, footer column separators, card chrome. The brand's elevated cards use an *inset* 1 px ring (`box-shadow: 0 0 0 1px rgba(0,0,0,0.08) inset`) instead of a `border` property — keeps the box-sizing math clean and lets the ring stack with drop shadows.

**Shadow system.** Five levels, all stacked. Never a single 8-px-blur drop. The signature pattern is the inset hairline + two soft drops:

```
0px 1px 1px #00000005,
0px 2px 2px #0000000a,
inset 0 0 0 1px #00000014
```

The brand reads calmer than Material because of this. *If you find yourself reaching for a `box-shadow: 0 4px 12px rgba(0,0,0,0.15)` you are not on brand.*

**Protection gradients / capsules.** The brand does not use vignettes or protection-gradients on top of imagery; the gradient is the imagery. Text always sits on flat white or flat ink — never on top of the mesh.

**Layout rules.** Sticky 64 px nav. 1400 px max content width (`--ds-page-width`); legacy surfaces still use the 1200 px (`--geist-page-width`) value. Horizontal gutters of 24 px desktop / 16 px mobile. The brand's `--geist-gap` is exactly 24 px; almost every "spacing between siblings" decision lands on 24 px.

**Transparency & blur.** Used sparingly. The nav uses `backdrop-filter: blur(12px)` when stuck at the top of the page (`background: rgba(255,255,255,0.8)`). Modals get a `rgba(0,0,0,0.32)` overlay scrim. Nothing else is translucent.

**Imagery vibe.** Cool / neutral. Screenshots of the product render on flat white backgrounds. Customer logos always rendered monochrome (single-color, never full-color marketing logos). No grain, no warm photography, no human photography. The brand's visual world is *the screenshot, the terminal, the gradient* — nothing else.

**Corner radii.** The official Geist scale is tight — **6 px** (`--r-sm`) for everyday controls, inputs, buttons; **12 px** (`--r-md`) for menus and modals; **16 px** (`--r-lg`) for fullscreen surfaces; **9999 px** (`--r-full`) for pills, avatars, circular controls. Keep one radius family per view — don't mix rounded and sharp corners.

  * Marketing-surface extensions (vercel.com only): 100 px (`--r-pill`) hero CTAs, 64 px (`--r-pill-sm`) tab pills. **Never** pair a marketing pill with the 6 px in-product scale on the same screen — pick a scale and stay there.

**Cards.** White fill (`--canvas`), 8 px radius (marketing) or 6 px (in-app), padded 16–32 px depending on density, lifted by the Level 2–4 stacked shadow above. The inset hairline ring is non-negotiable — without it the card edge disappears against the near-white page.

---

## Iconography

Vercel's icons are **monochrome line-art SVGs**, 1.5 px stroke, 16 / 20 / 24 px box. The brand uses an in-house icon set close in spirit to Lucide / Heroicons (outline variant); we substitute **Lucide** here for accessibility — load from `https://unpkg.com/lucide@latest`.

* **Stroke weight:** 1.5 px at 16 px, scales linearly.
* **Style:** outline only. The brand uses filled icons only for the Vercel triangle mark (and brand mark variants).
* **Color:** icons inherit `currentColor`. They are *never* tinted to an accent color in marketing chrome. In-app, status icons can take `--success` `--warning` `--error` semantic colors.
* **Emoji:** never used. Even in dashboard empty-states, the brand prefers a monochrome SVG illustration over an emoji-led empty state.
* **Unicode glyphs:** the thin arrow (→) appears in CTA labels ("Start Deploying →"). Otherwise no unicode chars stand in for icons.

**Logo system.**
  * `assets/vercel-logo-mark.svg` — the black equilateral triangle. The canonical brand mark, used at 24 px in nav and at scale on dark surfaces.
  * `assets/vercel-logo-wordmark.svg` — the triangle + "Vercel" wordmark, used in the footer and on press surfaces.

Logo color is single-tone — always either pure black (`#171717`) or pure white (`#ffffff`). The triangle is never gradient-filled, never multi-color.

---

## Fonts

Geist and Geist Mono are **open-source** and loaded directly from Google Fonts — no substitution:

* **Geist** (400 / 500 / 600) — every display, body, button, link, and label. Weight 600 is the display ceiling.
* **Geist Mono** (400 / 500) — terminal mockups, code blocks, filenames, mono eyebrows.
* **Space Grotesk** — loaded as the editorial fallback per the spec; rarely renders as the primary face.

Motion uses the Geist easing `cubic-bezier(0.175, 0.885, 0.32, 1.1)` — `--ease-geist` (150 ms state / 200 ms popover / 300 ms overlay). Focus shows the two-layer ring `--focus-ring` (2 px surface gap + 2 px `blue-700`).
