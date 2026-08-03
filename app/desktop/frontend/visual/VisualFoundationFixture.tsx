import {
  AgentAppShell,
  AgentComposerSurface,
  AgentContentCard,
  AgentRow,
  AgentStatusPill,
  AgentSurfaceHeader,
} from "@/ui/agent";
import { Button, IconButton } from "@/ui";

interface VisualFoundationFixtureProps {
  sidebarOpen: boolean;
}

const SESSION_ROWS = [
  { id: "api", title: "Refine runtime protocol", meta: "now", active: true },
  { id: "visual", title: "Polish desktop shell", meta: "12m", active: false },
  { id: "tests", title: "Close conformance gaps", meta: "1h", active: false },
] as const;

function WorkIndexFixture() {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <AgentSurfaceHeader divider={false} className="pl-[78px]">
        <span className="text-ui-md font-semibold text-fg">Lynx</span>
        <span className="min-w-2 flex-1" />
        <IconButton icon="search" size="sm" aria-label="Search" />
        <IconButton icon="edit" size="sm" aria-label="New session" />
      </AgentSurfaceHeader>
      <div className="panel-scroll flex min-h-0 flex-1 flex-col px-2 pt-2">
        <div className="px-2 pb-1 text-ui-xs font-medium tracking-wide text-fg-faint uppercase">
          Work index
        </div>
        <AgentRow icon="folder" trailing={<span className="tabular-nums">3</span>}>
          lynx
        </AgentRow>
        <div className="flex flex-col gap-0.5">
          {SESSION_ROWS.map((session) => (
            <AgentRow
              key={session.id}
              icon="chat"
              indent="nested"
              active={session.active}
              trailing={
                <span className="font-mono text-ui-xs text-fg-faint tabular-nums">
                  {session.meta}
                </span>
              }
            >
              {session.title}
            </AgentRow>
          ))}
        </div>
      </div>
      <div className="flex items-center gap-1 px-2 pb-2">
        <IconButton icon="settings" size="sm" aria-label="Settings" />
        <span className="text-ui-sm text-fg-muted">Visual fixture</span>
      </div>
    </div>
  );
}

function ComposerFixture() {
  return (
    <AgentComposerSurface data-testid="composer" className="relative">
      <div className="min-h-20 px-[var(--density-composer-editor-start)] pt-[var(--density-composer-editor-top)] pb-[var(--density-composer-editor-bottom)] text-ui-md leading-relaxed text-fg">
        Ask Lynx to inspect, change, or explain this workspace…
      </div>
      <div className="flex items-center gap-1 px-[var(--density-composer-footer)] pb-[var(--density-composer-footer)] pl-[var(--density-composer-footer)]">
        <Button variant="ghost" size="xs">
          Agent
        </Button>
        <Button variant="ghost" size="xs">
          Auto
        </Button>
        <span className="min-w-2 flex-1" />
        <IconButton icon="arrow-up" size="md" aria-label="Send" className="bg-cta text-cta-text" />
      </div>
    </AgentComposerSurface>
  );
}

function FoundationSurface({ sidebarOpen }: { sidebarOpen: boolean }) {
  return (
    <AgentContentCard label="Visual foundation" data-testid="content-card">
      <AgentSurfaceHeader windowCorner>
        <IconButton icon="panel-l" size="sm" aria-label="Toggle work index" />
        <span className="font-mono text-ui-sm text-fg-faint">lynx</span>
        <span className="text-ui-md text-fg-faint">/</span>
        <span className="truncate text-ui-md font-semibold text-fg">Visual foundation</span>
        <AgentStatusPill tone="neutral">Ready</AgentStatusPill>
        <span className="min-w-2 flex-1" />
        <IconButton icon="panel-r" size="sm" aria-label="Open context dock" />
      </AgentSurfaceHeader>

      <div className="panel-scroll flex min-h-0 flex-1 flex-col">
        <div className="mx-auto flex w-full max-w-[var(--content-max)] flex-1 flex-col px-[var(--density-column-gutter)] pt-10 sm:px-[var(--density-column-gutter-wide)]">
          <div className="text-ui-xs font-medium tracking-wide text-fg-faint uppercase">
            Deterministic visual fixture
          </div>
          <h1 className="mt-2 text-display-lg font-semibold leading-tight text-fg">
            One visual language, one source of truth.
          </h1>
          <p className="mt-3 max-w-[62ch] text-ui-md leading-relaxed text-fg-soft">
            Production primitives render this fixture. The route owns only stable test content,
            viewport, locale, and appearance—never a parallel business model.
          </p>

          <div className="mt-8 grid grid-cols-2 gap-4">
            <section className="rounded-lg border border-field bg-surface p-4">
              <div className="text-ui-sm font-medium text-fg-faint">TYPE LADDER</div>
              <div className="mt-3 flex items-end gap-3 text-fg">
                <span className="text-ui-2xs">9</span>
                <span className="text-ui-xs">10</span>
                <span className="text-ui-sm">11</span>
                <span className="text-ui-md">12</span>
                <span className="text-ui-md">13</span>
                <code className="font-mono text-code">code 11</code>
              </div>
            </section>
            <section className="rounded-lg border border-field bg-surface p-4">
              <div className="text-ui-sm font-medium text-fg-faint">SURFACE ROLES</div>
              <div className="mt-3 flex items-center gap-2">
                <Button variant="primary" size="sm">
                  Continue
                </Button>
                <Button variant="outline" size="sm">
                  Review
                </Button>
                <AgentStatusPill tone="warning">Needs input</AgentStatusPill>
              </div>
            </section>
          </div>

          <div className="min-h-8 flex-1" />
          <ComposerFixture />
          <div className="h-4 shrink-0" />
        </div>
      </div>
      <output className="sr-only" data-testid="sidebar-state">
        {sidebarOpen ? "expanded" : "collapsed"}
      </output>
    </AgentContentCard>
  );
}

export function VisualFoundationFixture({ sidebarOpen }: VisualFoundationFixtureProps) {
  return (
    <AgentAppShell
      sidebarLabel="Work index"
      sidebarResizeLabel="Resize the work index"
      sidebarOpen={sidebarOpen}
      sidebarWidth={256}
      onResize={() => undefined}
      onSidebarToggle={() => undefined}
      sidebarExpandLabel="Expand the foundation fixture sidebar"
      sidebarCollapseLabel="Collapse the foundation fixture sidebar"
      sidebar={<WorkIndexFixture />}
      main={<FoundationSurface sidebarOpen={sidebarOpen} />}
    />
  );
}
