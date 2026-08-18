# Visual fixtures

This is a test-only Vite entry for deterministic screenshot and interaction
checks. It is intentionally separate from the production router and Wails
bootstrap.

Rules:

- import production components, styles, selectors, projections, and view models;
- freeze only test inputs such as viewport, locale, clock, theme, and canonical
  protocol snapshots;
- never add a production debug route, fixture-only business branch, or parallel
  presentation state model;
- future Agent fixtures must start from `AgentSessionSnapshot` or `RunEvent` and
  use the production fold/projection before seeding a test adapter;
- keep fixtures inert unless an interaction is the subject of the test.

The `foundation` fixture freezes geometry and primitive roles. The `shell`
fixture installs test-only data providers, then renders the production sidebar
plugins, application projections, workspace navigation port, and shell
primitives for populated, empty, loading, error, collapsed, resized, and Retina
states. The `agent` fixture is backed by canonical `AgentSessionSnapshot`
values and the production `projectAgentSessionSnapshot` fold; its state selector
covers empty, idle, Running, Waiting/HITL, terminal, error, delegated-tree, and
long-content cases. The `workspace` fixture starts from the canonical Agent
snapshot installer, registers the real workspace views and Settings pane
plugins, and supplies only deterministic data providers. It covers per-density
dock widths, navigation identity, diff loading/empty/error states, and the
production Settings surface without a parallel presentation model.

`closure.visual.spec.ts` adds cross-surface release evidence: WCAG A/AA audits,
keyboard-only traversal, IME composition, real 44 px coarse-pointer targets,
the production clipboard path, OS and application motion preferences, the
maximum 18 px UI type setting, and DPR 2 hairlines. Its goldens use a `0.05`
per-pixel threshold so subtle ink regressions remain visible; geometry and
contrast also have semantic assertions.

`webkit.visual.spec.ts` is a compatibility smoke suite for the rendering engine
closest to Wails' macOS WKWebView. It validates shell focus handoff, Agent HITL
keyboard operation, CJK and syntax-highlighted long content, review geometry,
and Settings menu focus return. It deliberately has no WebKit goldens:
engine-specific font rasterisation must not create a second pixel baseline.
Chromium owns deterministic screenshot comparison; WebKit owns CSS, layout,
focus, and event compatibility.

Run `npm run visual:dev` for inspection and `npm run visual:test` for regression
checks. Update reviewed baselines with `npm run visual:test:update`.

Useful routes:

- `/visual/?theme=light&sidebar=expanded`
- `/visual/?fixture=shell&theme=light&state=populated&sidebar=expanded`
- `/visual/?fixture=shell&theme=dark&state=error`
- `/visual/?fixture=agent&theme=dark&state=waiting`
- `/visual/?fixture=workspace&theme=light&state=dock-light`
- `/visual/?fixture=workspace&theme=dark&state=dock-review`
- `/visual/?fixture=workspace&theme=light&state=dock-loading`
- `/visual/?fixture=workspace&theme=dark&state=settings`
- `/visual/?fixture=agent&theme=light&state=long-content&font-size=18`
- `/visual/?fixture=shell&theme=light&state=populated&motion=full`
