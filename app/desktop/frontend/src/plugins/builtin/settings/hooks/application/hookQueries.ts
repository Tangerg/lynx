import { createParameterizedDataQuery } from "@/lib/data/dataQuery";

export interface HookReadModel {
  event: string;
  matcher?: string;
  command?: string;
  inject?: string;
  timeoutMs?: number;
  scope: "global" | "project";
  source: string;
  active: boolean;
}

export interface HookListReadModel {
  hooks: HookReadModel[];
  projectRoot?: string;
  projectTrusted: boolean;
}

export interface HooksQuery {
  cwd?: string;
}

export const HOOKS_KEY = "hooks";
export const useHooks = createParameterizedDataQuery<HooksQuery, HookListReadModel>(HOOKS_KEY);
