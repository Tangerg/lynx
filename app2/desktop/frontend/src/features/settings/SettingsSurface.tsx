import { useEffect, useRef, useState } from "react";
import type { LucideIcon } from "lucide-react";
import {
  CalendarClock,
  ChartNoAxesColumnIncreasing,
  ChevronLeft,
  Command,
  Palette,
  Plug,
  Server,
  ShieldCheck,
  Search,
  Sparkles,
  Workflow,
} from "lucide-react";

import type { RuntimeConnection, WorkspaceRef } from "@lyra/runtime-contract";

import { useLocalization, type MessageKey } from "../localization/Localization";
import { MCPSettings } from "./MCPSettings";
import { ApprovalSettings } from "./ApprovalSettings";
import { ProviderModelSettings } from "./ProviderModelSettings";
import { ScheduleSettings } from "./ScheduleSettings";
import { HookSettings } from "./HookSettings";
import { UsageSettings } from "./UsageSettings";
import { AppearanceSettings } from "./AppearanceSettings";
import { RuntimeSettings } from "./RuntimeSettings";
import { KeyboardSettings } from "./KeyboardSettings";
import { ariaKeyShortcuts, commandByID } from "../shell/commandCatalog";
import { Icon } from "../shell/Icon";

interface SettingsSurfaceProps {
  connection: RuntimeConnection;
  sessionId?: string;
  workspace?: WorkspaceRef;
  onClose: () => void;
  onOpenSession: (sessionId: string) => void;
  onRuntimeChanged: () => Promise<void>;
  initialPage?: SettingsPage;
}

const settingsPages = [
  {
    id: "appearance",
    icon: Palette,
    title: "settings.page.appearance.title",
    description: "settings.page.appearance.description",
  },
  {
    id: "runtime",
    icon: Server,
    title: "settings.page.runtime.title",
    description: "settings.page.runtime.description",
  },
  {
    id: "providers",
    icon: Sparkles,
    title: "settings.page.providers.title",
    description: "settings.page.providers.description",
  },
  {
    id: "mcp",
    icon: Plug,
    title: "settings.page.mcp.title",
    description: "settings.page.mcp.description",
  },
  {
    id: "approvals",
    icon: ShieldCheck,
    title: "settings.page.approvals.title",
    description: "settings.page.approvals.description",
  },
  {
    id: "schedules",
    icon: CalendarClock,
    title: "settings.page.schedules.title",
    description: "settings.page.schedules.description",
  },
  {
    id: "hooks",
    icon: Workflow,
    title: "settings.page.hooks.title",
    description: "settings.page.hooks.description",
  },
  {
    id: "keyboard",
    icon: Command,
    title: "settings.page.keyboard.title",
    description: "settings.page.keyboard.description",
  },
  {
    id: "usage",
    icon: ChartNoAxesColumnIncreasing,
    title: "settings.page.usage.title",
    description: "settings.page.usage.description",
  },
] as const satisfies ReadonlyArray<{
  id: string;
  icon: LucideIcon;
  title: MessageKey;
  description: MessageKey;
}>;

export type SettingsPage = (typeof settingsPages)[number]["id"];

export function SettingsSurface(props: SettingsSurfaceProps) {
  const { t } = useLocalization();
  const [page, setPage] = useState<SettingsPage>(
    props.initialPage ?? "appearance",
  );
  const [search, setSearch] = useState("");
  const surface = useRef<HTMLElement>(null);
  const close = useRef(props.onClose);
  close.current = props.onClose;

  useEffect(() => {
    if (props.initialPage !== undefined) setPage(props.initialPage);
  }, [props.initialPage]);

  useEffect(() => {
    surface.current?.focus({ preventScroll: true });
    const handleDialogKey = (event: KeyboardEvent) => {
      if (event.isComposing || event.keyCode === 229) return;
      if (event.key === "Escape") {
        event.preventDefault();
        close.current();
        return;
      }
      if (event.key !== "Tab") return;
      const controls = [
        ...(surface.current?.querySelectorAll<HTMLElement>(
          'button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [href], [tabindex]:not([tabindex="-1"])',
        ) ?? []),
      ];
      const first = controls[0];
      const last = controls.at(-1);
      if (first === undefined || last === undefined) return;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    window.addEventListener("keydown", handleDialogKey);
    return () => {
      window.removeEventListener("keydown", handleDialogKey);
      window.requestAnimationFrame(() => {
        const trigger = document.querySelector<HTMLButtonElement>(
          'button[data-settings-trigger="true"]',
        );
        trigger?.focus();
      });
    };
  }, []);
  const activePage =
    settingsPages.find((candidate) => candidate.id === page) ??
    settingsPages[0];
  const normalizedSearch = search.trim().toLocaleLowerCase();
  const visiblePages = settingsPages.filter(
    (candidate) =>
      normalizedSearch === "" ||
      t(candidate.title).toLocaleLowerCase().includes(normalizedSearch) ||
      t(candidate.description).toLocaleLowerCase().includes(normalizedSearch),
  );

  return (
    <section
      ref={surface}
      className="settings-surface"
      role="dialog"
      aria-modal="true"
      aria-labelledby="settings-title"
      tabIndex={-1}
    >
      <aside className="settings-nav">
        <header>
          <button
            className="settings-back"
            type="button"
            aria-label={t("settings.close")}
            aria-keyshortcuts={ariaKeyShortcuts(
              commandByID("workspace.close").shortcut,
            )}
            onClick={props.onClose}
          >
            <Icon glyph={ChevronLeft} size="sm" />
            {t("settings.close")}
          </button>
        </header>
        <label className="settings-search">
          <Icon glyph={Search} size="sm" />
          <span className="sr-only">{t("settings.search")}</span>
          <input
            type="search"
            value={search}
            placeholder={t("settings.search")}
            autoComplete="off"
            onChange={(event) => setSearch(event.currentTarget.value)}
          />
        </label>
        <nav aria-label={t("settings.sections")}>
          {visiblePages.map((candidate) => (
            <button
              key={candidate.id}
              type="button"
              aria-current={page === candidate.id ? "page" : undefined}
              onClick={() => setPage(candidate.id)}
            >
              <Icon glyph={candidate.icon} size="sm" />
              {t(candidate.title)}
            </button>
          ))}
        </nav>
        <p>{t("settings.authorityNote")}</p>
      </aside>
      <div className="settings-content">
        <header className="settings-heading">
          <div>
            <h1 id="settings-title">{t(activePage.title)}</h1>
            <p>{t(activePage.description)}</p>
          </div>
        </header>
        <div className="settings-scroll">
          {page === "appearance" ? (
            <AppearanceSettings />
          ) : page === "runtime" ? (
            <RuntimeSettings onRuntimeChanged={props.onRuntimeChanged} />
          ) : page === "providers" ? (
            <ProviderModelSettings connection={props.connection} />
          ) : page === "mcp" ? (
            <MCPSettings connection={props.connection} />
          ) : page === "approvals" ? (
            <ApprovalSettings
              connection={props.connection}
              sessionId={props.sessionId}
            />
          ) : page === "schedules" ? (
            <ScheduleSettings
              connection={props.connection}
              onOpenSession={props.onOpenSession}
            />
          ) : page === "hooks" ? (
            <HookSettings
              connection={props.connection}
              workspace={props.workspace}
            />
          ) : page === "keyboard" ? (
            <KeyboardSettings />
          ) : (
            <UsageSettings
              connection={props.connection}
              sessionId={props.sessionId}
            />
          )}
        </div>
      </div>
    </section>
  );
}
