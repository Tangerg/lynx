# UI kit — HeroUI documentation site

A working recreation of `heroui.com/docs/react/components/*`, the surface HeroUI ships its own
design language on. Built entirely from this design system's components — no bespoke primitives.

## Files
| File | What it holds |
| --- | --- |
| `index.html` | The interactive kit. Sidebar filter, component switching, light/dark toggle, ⌘K palette. |
| `DocsShell.jsx` | `DocsTopBar`, `DocsSidebar`, `DocsToc` — the chrome. |
| `ComponentPage.jsx` | `ComponentPage` — title block, Preview/Code tabs, CSS class list, props table, prev/next. |
| `SearchPalette.jsx` | `SearchPalette` — the ⌘K overlay. |

## What's interactive
- Click any component in the left rail (or filter it) to swap the page.
- **⌘K / Ctrl-K** opens the search palette; Esc closes it.
- The sun/moon segmented control swaps `class="light|dark"` + `data-theme` on `<html>` — the same
  mechanism HeroUI documents, with every token recalculating.
- Preview/Code tabs on each component page.

## Fidelity notes
- Four component pages carry real content (Button, Switch, Alert, Card) transcribed from the
  published docs. Every other entry renders an explicit stub rather than invented copy.
- The header's star count, version chip and nav labels match the live site at v3.2.2.
- Not recreated: MDX prose bodies, the theme-preset gallery, the Pro banner artwork, and the
  Web/Native platform switcher's Native side.
