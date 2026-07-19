import { useHooks, type HookReadModel } from "./hookQueries";

export type HookConfig = HookReadModel;

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
          timeoutMs: hook.timeoutMs,
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
