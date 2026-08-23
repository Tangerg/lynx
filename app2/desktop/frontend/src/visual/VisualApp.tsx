import { useState } from "react";
import {
  Bot,
  CalendarClock,
  ChevronDown,
  Folder,
  PanelLeft,
  PanelRight,
  Settings,
  Wrench,
  X,
} from "lucide-react";

import {
  protocolVersion,
  type Item,
  type RunSummary,
  type RuntimeConnection,
  type Session,
} from "@lyra/runtime-contract";

import {
  Composer,
  emptyComposerDraft,
  type ComposerDraft,
} from "../features/agent/Composer";
import { AgentNarrative } from "../features/agent/AgentNarrative";
import { useLocalization } from "../features/localization/Localization";
import { NewSessionMenu } from "../features/sessions/NewSessionMenu";
import { SessionIndex } from "../features/sessions/SessionIndex";
import { SettingsSurface } from "../features/settings/SettingsSurface";
import { Icon } from "../features/shell/Icon";

interface VisualAppProps {
  initialSurface: "workspace" | "settings";
  sidebarOpen: boolean;
  dockOpen: boolean;
}

const now = "2026-08-24T10:30:00.000Z";
const connection: RuntimeConnection = {
  endpoint: "http://127.0.0.1:61432",
  bearerToken: "visual-acceptance-only",
  instanceId: "runtime-local",
  protocolVersion,
  idempotencyNamespace: "visual-acceptance",
  generation: 1,
};

const sessions: Session[] = [
  session("session-app2", "重写 app2 runtime 与 desktop", "running", true),
  session("session-ui", "统一 Desktop 视觉系统", "idle"),
  session("session-contract", "审阅 Lyra Protocol 契约", "idle"),
  {
    ...session("session-docs", "整理迁移验收文档", "waiting"),
    workspace: {
      ref: { path: "/Users/tangerg/Desktop/dougong" },
      projectRoot: "/Users/tangerg/Desktop/dougong",
      availability: "available",
    },
  },
];

const runs: RunSummary[] = [
  {
    id: "run-1",
    sessionId: "session-app2",
    provider: "openai",
    model: "gpt-5.6",
    status: "running",
    createdAt: now,
  },
];

const items: Item[] = [
  {
    id: "item-user",
    runId: "run-1",
    status: "completed",
    createdAt: now,
    type: "userMessage",
    content: [
      {
        type: "text",
        text: "把 Desktop 做得和旧版一致，同时保留 app2 更干净的架构。",
      },
    ],
  },
  {
    id: "item-agent",
    runId: "run-1",
    status: "completed",
    createdAt: "2026-08-24T10:30:08.000Z",
    type: "agentMessage",
    phase: "finalAnswer",
    content: [
      {
        type: "text",
        text: [
          "视觉基础已经重新收敛。侧栏、阅读区与输入框现在遵循同一套密度和层级规则。",
          "",
          "- Lyra Protocol 仍然是唯一协议真相",
          "- 主阅读面保持 760px，长内容不会漂散",
          "- 设置页与工作区共享字体、色彩和交互语法",
          "",
          "接下来会逐屏检查运行态、等待态和错误态。",
        ].join("\n"),
      },
    ],
  },
];

export function VisualApp(props: VisualAppProps) {
  const { t } = useLocalization();
  const [selectedID, setSelectedID] = useState("session-app2");
  const [draft, setDraft] = useState<ComposerDraft>(emptyComposerDraft);
  const [sidebarOpen, setSidebarOpen] = useState(props.sidebarOpen);
  const [dockOpen, setDockOpen] = useState(props.dockOpen);
  const [settingsOpen, setSettingsOpen] = useState(
    props.initialSurface === "settings",
  );
  const selected = sessions.find((candidate) => candidate.id === selectedID);

  return (
    <>
      <main
        className="app-shell"
        data-sidebar={sidebarOpen ? "expanded" : "collapsed"}
        data-dock={dockOpen ? "expanded" : "collapsed"}
        aria-hidden={settingsOpen || undefined}
        inert={settingsOpen}
      >
        <aside className="work-index" aria-label={t("shell.workIndex")}>
          <header className="panel-header window-drag">
            <div className="work-index-actions window-no-drag">
              <button
                className="icon-action"
                type="button"
                aria-label={t("shell.workIndex")}
                onClick={() => setSidebarOpen(false)}
              >
                <Icon glyph={PanelLeft} size="sm" />
              </button>
            </div>
          </header>
          <SessionIndex
            sessions={sessions}
            selectedId={selectedID}
            pending={false}
            error={undefined}
            actionPending={false}
            hasMore={false}
            loadingMore={false}
            onSelect={setSelectedID}
            onUpdate={async (source) => source}
            onRemove={async () => undefined}
            onFork={async () => undefined}
            onExport={async () => undefined}
            onRetry={() => undefined}
            onLoadMore={() => undefined}
            headerActions={
              <nav className="work-index-primary-actions">
                <NewSessionMenu
                  pending={false}
                  defaultWorkspace="/Users/tangerg/Desktop/lynx"
                  onCreate={async () => sessions[0]!}
                  onImport={async () => sessions[0]}
                />
                <button type="button">
                  <Icon glyph={CalendarClock} size="sm" />
                  {t("settings.page.schedules.title")}
                </button>
                <button type="button">
                  <Icon glyph={Wrench} size="sm" />
                  {t("settings.page.mcp.title")}
                </button>
              </nav>
            }
          />
          <footer className="work-index-footer">
            <button type="button" onClick={() => setSettingsOpen(true)}>
              <Icon glyph={Settings} size="sm" />
              {t("shell.settings")}
            </button>
          </footer>
        </aside>

        <section className="narrative" aria-label={t("shell.agentNarrative")}>
          <header className="narrative-header window-drag">
            <div className="narrative-heading">
              {!sidebarOpen ? (
                <button
                  className="icon-action window-no-drag"
                  type="button"
                  onClick={() => setSidebarOpen(true)}
                >
                  <Icon glyph={PanelLeft} size="sm" />
                </button>
              ) : null}
              <span>
                <Icon glyph={Folder} size="xs" />
                lynx
              </span>
              <i aria-hidden="true">/</i>
              <h2>{selected?.title}</h2>
            </div>
            <div className="narrative-tools window-no-drag">
              <span className="context-gauge">
                <span>
                  <i style={{ width: "38%" }} />
                </span>
                <b>38%</b>
              </span>
              <span className="online-pill">
                <span />
                {t("shell.liveUpdates")}
              </span>
              <button
                className="icon-action"
                type="button"
                aria-label={t("shell.contextDock")}
                onClick={() => setDockOpen((current) => !current)}
              >
                <Icon glyph={PanelRight} size="sm" />
              </button>
            </div>
          </header>
          <div className="narrative-content">
            <AgentNarrative
              sessionTitle={selected?.title ?? ""}
              items={items}
              runs={runs}
              liveToolOutputs={{}}
              interrupts={[]}
              pending={false}
              interruptPending={false}
              onResume={async () => undefined}
              onCancelRun={async () => undefined}
              onFeedback={async () => undefined}
              hasOlderHistory={false}
              historyPending={false}
              onLoadOlderHistory={async () => undefined}
              onForkFrom={async () => undefined}
              onRollback={async () => undefined}
              searchRequest={0}
            />
            <Composer
              sessionId="session-app2"
              draft={draft}
              recipes={[]}
              pending={false}
              attachmentPolicy="multimodal"
              onChange={(update) => setDraft(update)}
              onSend={async () => undefined}
              onStop={async () => undefined}
            >
              <button
                className="composer-tool model-picker-trigger"
                type="button"
              >
                <Icon glyph={Bot} size="sm" />
                gpt-5.6
                <Icon glyph={ChevronDown} size="xs" />
              </button>
            </Composer>
          </div>
        </section>

        <aside className="context-dock" aria-label={t("shell.contextDock")}>
          <header className="panel-header window-drag">
            <h2>{t("shell.session")}</h2>
            <button
              className="icon-action window-no-drag"
              type="button"
              onClick={() => setDockOpen(false)}
            >
              <Icon glyph={X} size="sm" />
            </button>
          </header>
          <nav className="context-dock-tabs">
            <button type="button" aria-selected="true">
              Overview
            </button>
            <button type="button">Resources</button>
          </nav>
          <section className="session-context">
            <div className="identity-card">
              <span>Workspace</span>
              <code>/Users/tangerg/Desktop/lynx</code>
            </div>
            <dl className="runtime-facts">
              <div>
                <dt>Runtime</dt>
                <dd>local · live</dd>
              </div>
              <div>
                <dt>Model</dt>
                <dd>gpt-5.6</dd>
              </div>
            </dl>
          </section>
        </aside>
      </main>

      {settingsOpen ? (
        <SettingsSurface
          connection={connection}
          sessionId="session-app2"
          workspace={{ path: "/Users/tangerg/Desktop/lynx" }}
          onClose={() => setSettingsOpen(false)}
          onOpenSession={() => setSettingsOpen(false)}
          onRuntimeChanged={async () => undefined}
        />
      ) : null}
    </>
  );
}

function session(
  id: string,
  title: string,
  status: string,
  favorite = false,
): Session {
  return {
    id,
    title,
    status,
    provider: "openai",
    model: "gpt-5.6",
    workspace: {
      ref: { path: "/Users/tangerg/Desktop/lynx" },
      projectRoot: "/Users/tangerg/Desktop/lynx",
      availability: "available",
    },
    createdAt: now,
    updatedAt: now,
    favorite,
    revision: 1,
  };
}
