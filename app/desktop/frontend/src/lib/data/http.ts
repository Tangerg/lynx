import type { KyInstance } from "ky";
import ky from "ky";
import { AGUI_BASE } from "@/main/config";
import { getConfig } from "@/plugins/sdk/config";
import { listRpcAfterHooks, listRpcBeforeHooks } from "@/plugins/sdk/selectors";

// First-paint base URL is owned by `main/config`. Plugins (typically
// `lyra.builtin.default-config`) can override at runtime via
// `host.config.set("api.baseUrl", "...")`. The override is read on
// every request so a runtime change takes effect immediately.

function activeBase(): string {
  return getConfig<string>("api.baseUrl") ?? AGUI_BASE;
}

// Thread the request through every before-hook: each may return a
// replacement Request (or void to leave it). Returning on the first match
// skipped the remaining hooks AND fed them the original request, so two
// plugins couldn't both transform it (e.g. base rewrite + auth header).
// ky applies the same "remaining hooks see the updated request" rule.
const beforeRequest = async (state: { request: Request }) => {
  let req = state.request;
  let changed = false;
  for (const hook of listRpcBeforeHooks()) {
    const result = await hook(req);
    if (result instanceof Request) {
      req = result;
      changed = true;
    }
  }
  return changed ? req : undefined;
};
const afterResponse = async (state: { request: Request; response: Response }) => {
  let res = state.response;
  let changed = false;
  for (const hook of listRpcAfterHooks()) {
    const result = await hook(state.request, res);
    if (result instanceof Response) {
      res = result;
      changed = true;
    }
  }
  return changed ? res : undefined;
};

// Re-anchor `req` against `base`, preserving path + search + the original
// Request's headers/body. Returns `req` unchanged when already on the right
// host so this is cheap on the hot path.
function anchorOn(req: Request, base: string): Request {
  if (req.url.startsWith(base)) return req;
  const { pathname, search } = new URL(req.url);
  const baseSlash = base.endsWith("/") ? base : `${base}/`;
  const target = new URL(pathname.replace(/^\//, "") + search, baseSlash);
  return new Request(target.toString(), req);
}

const rewriteBase = (state: { request: Request }) => anchorOn(state.request, activeBase());

// Pre-configured ky instance for the local backend.
//
// `retry: 0` because react-query already handles retries — letting ky
// retry would double-retry on transient failures.
export const api: KyInstance = ky.create({
  baseUrl: AGUI_BASE,
  retry: 0,
  timeout: 30_000,
  headers: { Accept: "application/json" },
  hooks: {
    beforeRequest: [rewriteBase, beforeRequest],
    afterResponse: [afterResponse],
  },
});
