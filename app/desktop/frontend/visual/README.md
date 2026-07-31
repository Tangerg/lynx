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

The `foundation` fixture freezes geometry and primitive roles. The `agent`
fixture is backed by canonical `AgentSessionSnapshot` values and the production
`projectAgentSessionSnapshot` fold; its state selector covers empty, idle,
Running, Waiting/HITL, terminal, error, delegated-tree, and long-content cases.
The `workspace` fixture covers the production dock and settings primitives with
stable presentation-only content.

Run `npm run visual:dev` for inspection and `npm run visual:test` for regression
checks. Update reviewed baselines with `npm run visual:test:update`.

Useful routes:

- `/?theme=light&sidebar=expanded`
- `/?fixture=agent&theme=dark&state=waiting`
- `/?fixture=workspace&theme=light&view=dock`
- `/?fixture=workspace&theme=dark&view=settings`
