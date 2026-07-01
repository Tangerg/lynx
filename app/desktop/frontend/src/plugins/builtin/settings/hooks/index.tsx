// Built-in plugin: "Hooks" settings pane. Reviews the lifecycle hooks the
// runtime discovered for the active project (workspace.hooks.list) — global
// (~/.lyra) + project (<root>/.lyra) — and toggles whether the project's hooks
// are trusted to run (workspace.hooks.setTrust).
//
// Trust is the security seam: a cloned repo's hooks run shell commands, so they
// stay inert (shown dimmed + "inactive") until the user explicitly trusts the
// project here. Global hooks are always active. The pane is read-only over the
// hook definitions themselves — those live in hooks.json files the user edits
// directly; the GUI only audits them and grants/revokes project trust.

import { DataView, EmptyState, Icon, Switch } from "@/components/common";
import { isUnsupportedMethod, rpcErrorText } from "@/lib/rpcErrors";
import type { HookConfig } from "./application/hookConfig";
import { useHookConfigs } from "./application/hookConfig";
import { setHookTrust } from "./application/hookTrust";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { notifyError } from "@/lib/notify";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";

function HookRow({ h }: { h: HookConfig }) {
  const t = useT();
  return (
    <div
      className={cn(
        "rounded-lg border border-line-soft bg-canvas px-3 py-2.5",
        !h.active && "opacity-55",
      )}
    >
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3">
        <Icon
          name={h.scope === "global" ? "globe" : "folder"}
          size={14}
          className="text-fg-faint"
        />
        <div className="flex min-w-0 items-center gap-2">
          <span className="shrink-0 rounded-xs bg-surface-2 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-fg-muted">
            {h.event}
          </span>
          {h.matcher && (
            <span className="shrink-0 font-mono text-[11px] text-accent" title={t("hooks.matcher")}>
              {h.matcher}
            </span>
          )}
          <span
            className="min-w-0 flex-1 truncate font-mono text-[12px] text-fg"
            title={h.command || h.inject || h.source}
          >
            {h.command ? h.command : <span className="text-fg-muted italic">{h.inject}</span>}
          </span>
        </div>
        {!h.active ? (
          <span
            title={t("hooks.inactive.hint")}
            className="shrink-0 rounded-xs border border-warning/30 bg-warning/12 px-1.5 py-px text-[10px] font-semibold text-warning"
          >
            {t("hooks.inactive")}
          </span>
        ) : h.inject ? (
          <span className="shrink-0 text-[10px] font-semibold text-fg-faint">
            {t("hooks.kind.inject")}
          </span>
        ) : null}
      </div>
    </div>
  );
}

function HooksPane() {
  const t = useT();
  const cwd = useActiveSessionCwd();
  const { data, isLoading, isError, error } = useHookConfigs(cwd);

  if (isError && isUnsupportedMethod(error)) {
    return (
      <EmptyState
        icon="lightning"
        title={t("hooks.unavailable")}
        sub={t("hooks.unavailable.sub")}
      />
    );
  }

  const projectRoot = data?.projectRoot;

  const onTrust = async (trusted: boolean) => {
    if (!projectRoot) return;
    try {
      await setHookTrust(projectRoot, trusted);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("hooks.error.trust"));
    }
  };

  return (
    <div className="flex flex-col gap-3">
      <p className="text-[13px] leading-[1.5] text-fg-muted">{t("hooks.intro")}</p>

      {projectRoot && data?.hasProjectHooks && (
        <div className="flex items-center justify-between gap-3 rounded-lg border border-line-soft bg-canvas px-3 py-2.5">
          <div className="min-w-0">
            <div className="text-[14px] font-semibold text-fg">{t("hooks.trust")}</div>
            <div className="mt-0.5 text-[12px] leading-[1.45] text-fg-muted">
              {t("hooks.trust.sub")}
            </div>
            <div className="mt-1 truncate font-mono text-[11px] text-fg-faint" title={projectRoot}>
              {projectRoot}
            </div>
          </div>
          <Switch
            checked={data?.projectTrusted ?? false}
            onCheckedChange={(v) => void onTrust(v)}
            ariaLabel={t("hooks.trust.aria")}
          />
        </div>
      )}

      <DataView
        items={data?.hooks}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{ icon: "lightning", title: t("hooks.empty"), sub: t("hooks.empty.sub") }}
      >
        {(rows) => (
          <div className="flex flex-col gap-2">
            {rows.map((h, i) => (
              <HookRow key={`${h.source}:${h.event}:${i}`} h={h} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.hooks-pane",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SETTINGS_PANE, {
      id: "hooks",
      label: "settings.pane.hooks",
      group: "agent",
      icon: "lightning",
      // After MCP servers (56) — both extend "what runs around the agent";
      // hooks are the lifecycle-command surface.
      order: 57,
      component: HooksPane,
    });
  },
});
