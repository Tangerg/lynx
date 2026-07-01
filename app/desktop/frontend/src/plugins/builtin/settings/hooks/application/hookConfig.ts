import { useHooks } from "@/lib/data/queries";

export interface HookConfig {
  event: string;
  matcher?: string;
  command?: string;
  inject?: string;
  scope: "global" | "project";
  source: string;
  active: boolean;
}

export interface HookConfigList {
  hooks: HookConfig[];
  projectRoot?: string;
  projectTrusted: boolean;
  hasProjectHooks: boolean;
}

export function useHookConfigs(cwd?: string) {
  const query = useHooks({ cwd });
  const source = query.data;
  const data: HookConfigList | undefined = source
    ? {
        hooks: source.hooks.map((hook) => ({
          event: hook.event,
          matcher: hook.matcher,
          command: hook.command,
          inject: hook.inject,
          scope: hook.scope,
          source: hook.source,
          active: hook.active,
        })),
        projectRoot: source.projectRoot,
        projectTrusted: source.projectTrusted,
        hasProjectHooks: source.hooks.some((hook) => hook.scope === "project"),
      }
    : undefined;
  return { ...query, data };
}
