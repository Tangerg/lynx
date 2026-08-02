repo: heroui-inc/heroui
branch: v3

## Last sync
date: 2026-08-01T18:22:16.990Z

### Updated in this project
- Transcribed the full theme token set from `themes/default/variables.css` and `themes/shared/theme.css` into `tokens/`.
- Transcribed `components/button.css` verbatim — the reference for HeroUI's component CSS shape.
- Built 67 components across 11 groups from the published component inventory.
- Recreated the documentation site as an interactive UI kit.

## Screen map
| Project screen | Built from |
| --- | --- |
| `tokens/primitives.css`, `tokens/colors.css`, `tokens/shadows.css` | `packages/styles/themes/default/variables.css` |
| `tokens/radius.css`, `tokens/motion.css` | `packages/styles/themes/shared/theme.css` |
| `tokens/typography.css` | heroui.com Typography component specimen |
| `components/buttons/buttons.css` | `packages/styles/components/button.css` |
| `components/*/*.css` (other groups) | Reconstructed from token set + published BEM class lists |
| `ui_kits/docs/*` | heroui.com/docs/react/components/* |

## Notes
GitHub file tools were not connected during this build; sources were read from public
github.com blob pages and heroui.com. No commit sha was resolved — do not assume one.
