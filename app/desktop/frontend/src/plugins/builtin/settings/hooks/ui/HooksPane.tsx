// The "Hooks" settings pane. Reviews the lifecycle hooks the runtime
// discovered for the active project (hooks.list) — global
// (~/.lyra) + project (<root>/.lyra) — and toggles whether the project's hooks
// are trusted to run (hooks.setTrust).
//
// Trust is the security seam: a cloned repo's hooks run shell commands, so they
// stay inert (shown dimmed + "inactive") until the user explicitly trusts the
// project here. Global hooks are always active. The pane is read-only over the
// hook definitions themselves — those live in hooks.json files the user edits
// directly; the GUI only audits them and grants/revokes project trust.

import { DataView, EmptyState, Icon, Surface, Switch } from "@/ui";
import { isUnsupportedMethod, rpcErrorText } from "@/lib/rpcErrors";
import type { HookReadModel } from "../application/hookConfig";
import { useHookConfigs } from "../application/hookConfig";
import { hookTrustMutationWasRetired, setHookTrust } from "../application/hookTrust";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { useRef, useState } from "react";

function HookRow({ h }: { h: HookReadModel }) {
  const t = useT();
  return (
    <div
      className={cn(
        "grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 rounded-md px-3 py-2.5 transition-colors hover:bg-hover",
        !h.active && "opacity-55",
      )}
    >
      <Icon name={h.scope === "global" ? "globe" : "folder"} size="sm" className="text-fg-faint" />
      <div className="flex min-w-0 items-center gap-2">
        <span className="shrink-0 rounded-sm bg-surface-2 px-1.5 py-0.5 font-mono text-ui-xs font-medium text-fg-muted">
          {h.event}
        </span>
        {h.matcher && (
          <span className="shrink-0 font-mono text-ui-sm text-accent" title={t("hooks.matcher")}>
            {h.matcher}
          </span>
        )}
        <span
          className="min-w-0 flex-1 truncate font-mono text-ui-md text-fg"
          title={h.command || h.inject || h.source}
        >
          {h.command ? h.command : <span className="text-fg-muted italic">{h.inject}</span>}
        </span>
      </div>
      {!h.active ? (
        <span
          title={t("hooks.inactive.hint")}
          className="shrink-0 rounded-sm bg-warning-wash px-1.5 py-px text-ui-xs font-medium text-warning"
        >
          {t("hooks.inactive")}
        </span>
      ) : h.inject ? (
        <span className="shrink-0 text-ui-xs font-medium text-fg-faint">
          {t("hooks.kind.inject")}
        </span>
      ) : null}
    </div>
  );
}

export function HooksPane() {
  const t = useT();
  const [trusting, setTrusting] = useState(false);
  const trustingRef = useRef(false);
  const workspace = useActiveSessionWorkspace();
  const { data, isLoading, isError, error } = useHookConfigs(
    workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );

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
    if (!projectRoot || trustingRef.current) return;
    trustingRef.current = true;
    setTrusting(true);
    try {
      await setHookTrust(projectRoot, trusted);
    } catch (err) {
      if (hookTrustMutationWasRetired(err)) return;
      notifyError(rpcErrorText(err) ?? t("hooks.error.trust"));
    } finally {
      trustingRef.current = false;
      setTrusting(false);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <p className="text-ui-md leading-body text-fg-muted">{t("hooks.intro")}</p>

      {projectRoot && data?.hasProjectHooks && (
        <Surface className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="text-ui-md font-medium text-fg">{t("hooks.trust")}</div>
            <div className="mt-0.5 text-ui-md leading-body text-fg-muted">
              {t("hooks.trust.sub")}
            </div>
            <div className="mt-1.5 truncate font-mono text-ui-sm text-fg-faint" title={projectRoot}>
              {projectRoot}
            </div>
          </div>
          <Switch
            checked={data?.projectTrusted ?? false}
            disabled={trusting}
            onCheckedChange={(v) => void onTrust(v)}
            ariaLabel={t("hooks.trust.aria")}
          />
        </Surface>
      )}

      <DataView
        items={data?.hooks}
        isLoading={isLoading || workspace.status === "resolving"}
        isError={isError}
        skeletonCount={3}
        empty={{ icon: "lightning", title: t("hooks.empty"), sub: t("hooks.empty.sub") }}
      >
        {(rows) => (
          <div className="flex flex-col gap-0.5">
            {rows.map((h, i) => (
              <HookRow key={`${h.source}:${h.event}:${i}`} h={h} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}
