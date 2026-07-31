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
states. The `agent`
fixture is backed by canonical `AgentSessionSnapshot` values and the production
`projectAgentSessionSnapshot` fold; its state selector covers empty, idle,
Running, Waiting/HITL, terminal, error, delegated-tree, and long-content cases.
The `workspace` fixture starts from the canonical Agent snapshot installer,
registers the real workspace views and Settings pane plugins, and supplies only
deterministic data providers. It covers per-density dock widths, navigation
identity, diff loading/empty/error states, and the production Settings surface
without a parallel presentation model.

Run `npm run visual:dev` for inspection and `npm run visual:test` for regression
checks. Update reviewed baselines with `npm run visual:test:update`.

Useful routes:

- `/?theme=light&sidebar=expanded`
- `/?fixture=shell&theme=light&state=populated&sidebar=expanded`
- `/?fixture=shell&theme=dark&state=error`
- `/?fixture=agent&theme=dark&state=waiting`
- `/?fixture=workspace&theme=light&state=dock-light`
- `/?fixture=workspace&theme=dark&state=dock-review`
- `/?fixture=workspace&theme=light&state=dock-loading`
- `/?fixture=workspace&theme=dark&state=settings`
