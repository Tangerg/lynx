import { SettingRow } from "@/plugins/builtin/settings/public/SettingRow";
import {
  Button,
  Icon,
  IconButton,
  SearchField,
  Segmented,
  Surface,
  Switch,
  VerticalTabs,
} from "@/ui";
import {
  AgentAppShell,
  AgentContentCard,
  AgentContextDock,
  AgentDockTabs,
  AgentRow,
  AgentStatusPill,
  AgentSurfaceHeader,
} from "@/ui/agent";

export type VisualWorkspaceView = "dock" | "settings";

function DockSidebar() {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <AgentSurfaceHeader divider={false} className="pl-[78px]">
        <span className="text-ui-lg font-semibold text-fg">lynx</span>
      </AgentSurfaceHeader>
      <div className="flex flex-col gap-0.5 px-2 pt-2">
        <AgentRow icon="chat" active>
          Runtime protocol
        </AgentRow>
        <AgentRow icon="folder">app/runtime</AgentRow>
        <AgentRow icon="folder">app/desktop</AgentRow>
      </div>
    </div>
  );
}

function DiffPreview() {
  return (
    <div className="panel-scroll min-h-0 flex-1 font-mono text-code leading-relaxed text-fg-soft">
      <div className="sticky top-0 flex h-8 items-center bg-surface px-3 text-ui-sm text-fg-muted">
        session.go
        <span className="min-w-2 flex-1" />
        <span className="text-positive">+12</span>
        <span className="ml-2 text-negative">−4</span>
      </div>
      <div className="grid grid-cols-[36px_1fr] py-2">
        <span className="px-2 text-right text-fg-faint">48</span>
        <span className="px-2">
          func (session *Session) Start(ctx context.Context) error &#123;
        </span>
        <span className="px-2 text-right text-fg-faint">49</span>
        <span className="bg-negative-wash px-2 text-negative">
          − return session.store.Save(ctx, session)
        </span>
        <span className="px-2 text-right text-fg-faint">49</span>
        <span className="bg-positive-wash px-2 text-positive">
          + return session.records.Commit(ctx, session.snapshot())
        </span>
        <span className="px-2 text-right text-fg-faint">50</span>
        <span className="px-2">&#125;</span>
      </div>
    </div>
  );
}

function DockFixture() {
  return (
    <AgentAppShell
      sidebarLabel="Work index"
      sidebarOpen
      sidebarWidth={244}
      onResize={() => undefined}
      sidebar={<DockSidebar />}
      main={
        <AgentContentCard label="Context dock fixture" data-testid="workspace-dock">
          <div className="flex min-h-0 flex-1">
            <section className="flex min-w-0 flex-1 flex-col">
              <AgentSurfaceHeader>
                <span className="font-mono text-ui-sm text-fg-faint">lynx</span>
                <span className="text-ui-lg text-fg-faint">/</span>
                <span className="text-ui-lg font-semibold text-fg">Runtime protocol</span>
                <AgentStatusPill tone="running">Running</AgentStatusPill>
                <span className="min-w-2 flex-1" />
                <IconButton icon="panel-r" size="sm" aria-label="Close context dock" />
              </AgentSurfaceHeader>
              <div className="panel-scroll flex min-h-0 flex-1 flex-col">
                <div className="mx-auto w-full max-w-[var(--content-max)] px-[var(--density-column-gutter)] py-8 sm:px-[var(--density-column-gutter-wide)]">
                  <div className="rounded-lg bg-user-message px-4 py-3 text-ui-lg leading-body text-fg">
                    Keep the framework generic and move transaction policy to the app boundary.
                  </div>
                  <p className="mt-8 text-ui-lg leading-relaxed text-fg">
                    The application now owns idempotency, durable records, and atomic publication.
                    The framework exposes only execution primitives.
                  </p>
                  <div className="mt-4 flex gap-2">
                    <Button variant="outline" size="sm">
                      Review diff
                    </Button>
                    <Button variant="primary" size="sm">
                      Run checks
                    </Button>
                  </div>
                </div>
              </div>
            </section>
            <AgentContextDock style={{ width: 392, flex: "0 0 392px" }}>
              <AgentSurfaceHeader>
                <AgentDockTabs
                  tabs={[
                    { id: "diff", title: "Changes", icon: "diff", active: true },
                    { id: "file", title: "session.go", icon: "file" },
                    { id: "terminal", title: "Terminal", icon: "terminal" },
                  ]}
                />
                <IconButton icon="x" size="sm" aria-label="Close" />
              </AgentSurfaceHeader>
              <DiffPreview />
            </AgentContextDock>
          </div>
        </AgentContentCard>
      }
    />
  );
}

function SettingsFixture() {
  return (
    <div className="h-full" data-testid="workspace-settings">
      <VerticalTabs
        ariaLabel="Settings"
        value="appearance"
        onValueChange={() => undefined}
        railHeader={
          <div className="flex flex-col">
            <AgentSurfaceHeader divider={false} windowCorner aria-hidden />
            <div className="px-4 pb-4">
              <button
                type="button"
                className="mb-3 flex h-8 items-center gap-2 rounded-sm px-2 text-ui-lg font-medium text-fg-muted"
              >
                <Icon name="arrow-left" size={15} />
                Back to Lynx
              </button>
              <SearchField
                size="lg"
                value=""
                onValueChange={() => undefined}
                placeholder="Search settings"
                aria-label="Search settings"
              />
            </div>
          </div>
        }
        groups={[
          {
            id: "general",
            label: "General",
            items: [
              {
                id: "appearance",
                label: "Appearance",
                icon: "sun",
                content: (
                  <div>
                    <div className="flex items-center gap-3">
                      <div>
                        <h1 className="text-display-lg font-semibold text-fg">Appearance</h1>
                        <p className="mt-1 text-ui-lg text-fg-muted">
                          Tune the workspace without changing its information hierarchy.
                        </p>
                      </div>
                      <span className="min-w-2 flex-1" />
                      <AgentStatusPill tone="neutral">Saved</AgentStatusPill>
                    </div>
                    <Surface inset="none" className="mt-7">
                      <SettingRow
                        label="Theme"
                        sub="Choose the canvas and surface contrast for this workspace."
                      >
                        <Segmented
                          value="system"
                          options={[
                            { value: "light", label: "Light" },
                            { value: "dark", label: "Dark" },
                            { value: "system", label: "System" },
                          ]}
                          onChange={() => undefined}
                          ariaLabel="Theme"
                        />
                      </SettingRow>
                      <div className="h-px bg-divider" />
                      <SettingRow
                        label="Reduced motion"
                        sub="Use direct state changes when the system requests less motion."
                      >
                        <Switch
                          checked
                          onCheckedChange={() => undefined}
                          ariaLabel="Reduced motion"
                        />
                      </SettingRow>
                    </Surface>
                  </div>
                ),
              },
            ],
          },
          {
            id: "agent",
            label: "Agent",
            items: [
              { id: "models", label: "Models", icon: "bot", content: null },
              { id: "approvals", label: "Approvals", icon: "shield", content: null },
            ],
          },
          {
            id: "advanced",
            label: "Advanced",
            items: [{ id: "runtime", label: "Runtime", icon: "terminal", content: null }],
          },
        ]}
      />
    </div>
  );
}

export function VisualWorkspaceFixture({ view }: { view: VisualWorkspaceView }) {
  return view === "settings" ? <SettingsFixture /> : <DockFixture />;
}
