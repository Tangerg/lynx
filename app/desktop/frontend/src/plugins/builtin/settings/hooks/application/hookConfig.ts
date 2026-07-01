import type { HookInfo } from "@/rpc";
import { useHooks } from "@/lib/data/queries";

export type HookConfig = HookInfo;

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
        hooks: source.hooks,
        projectRoot: source.projectRoot,
        projectTrusted: source.projectTrusted,
        hasProjectHooks: source.hooks.some((hook) => hook.scope === "project"),
      }
    : undefined;
  return { ...query, data };
}
