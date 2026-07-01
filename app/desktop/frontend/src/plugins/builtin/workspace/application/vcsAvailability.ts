import { isErrorType } from "@/rpc";

export function isVcsUnavailable(error: unknown): boolean {
  return isErrorType(error, "vcs_unavailable");
}
